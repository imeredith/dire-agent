package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestTerminalEnvironmentEnablesTrueColorAndRemovesColorOptOuts(t *testing.T) {
	environment := terminalEnvironment([]string{
		"PATH=/usr/bin",
		"TERM=dumb",
		"COLORTERM=",
		"NO_COLOR=1",
		"FORCE_COLOR=0",
		"CLICOLOR=0",
		"DIRE_AGENT_PROJECT_ID=old",
		"GOAGENT_PROJECT_ID=legacy",
		"UNRELATED=kept",
	}, "project_test")

	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	for name, want := range map[string]string{
		"TERM":                  "xterm-256color",
		"COLORTERM":             "truecolor",
		"TERM_PROGRAM":          "dire-agent",
		"COLORFGBG":             "15;0",
		"CLICOLOR":              "1",
		"DIRE_AGENT_PROJECT_ID": "project_test",
		"GOAGENT_PROJECT_ID":    "project_test",
		"UNRELATED":             "kept",
	} {
		if got := values[name]; got != want {
			t.Fatalf("%s = %q, want %q; environment=%q", name, got, want, environment)
		}
	}
	for _, name := range []string{"NO_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE"} {
		if _, found := values[name]; found {
			t.Fatalf("%s must not reach the interactive PTY; environment=%q", name, environment)
		}
	}
}

func TestProjectLauncherCommandLinePreservesConfiguredArguments(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"literal;touch should-not-exist", "$(pwd)", "argument with spaces"}
	resolved, arguments, err := projectLauncherCommandLine(configuration.ProjectLauncher{
		ID: "direct", Label: "Direct", Kind: configuration.LauncherTerminal,
		Command: executable, Args: want,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved != executable || !reflect.DeepEqual(arguments, want) {
		t.Fatalf("command line = %q %#v, want %q %#v", resolved, arguments, executable, want)
	}
	arguments[0] = "mutated"
	if want[0] == "mutated" {
		t.Fatal("resolved arguments alias configuration")
	}
}

func TestProjectApplicationEnvironmentReplacesProjectID(t *testing.T) {
	environment := projectApplicationEnvironment([]string{
		"PATH=/usr/bin", "DIRE_AGENT_PROJECT_ID=old", "GOAGENT_PROJECT_ID=legacy", "OTHER=kept",
	}, "project_new")
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	if values["DIRE_AGENT_PROJECT_ID"] != "project_new" || values["GOAGENT_PROJECT_ID"] != "project_new" || values["OTHER"] != "kept" {
		t.Fatalf("environment = %#v", environment)
	}
}

func TestEnvironmentActionLauncherContract(t *testing.T) {
	ctx := context.Background()
	source := canonicalTestDirectory(t, t.TempDir())
	worktree := canonicalTestDirectory(t, t.TempDir())
	platform := currentEnvironmentPlatform()
	otherPlatform := "darwin"
	if platform == otherPlatform {
		otherPlatform = "linux"
	}
	actionScript := `printf '%s\n%s\n%s\n' "$PWD" "$CODEX_SOURCE_TREE_PATH" "$CODEX_WORKTREE_PATH" > action-result.txt`
	if platform == "win32" {
		actionScript = `@($PWD.Path, $env:CODEX_SOURCE_TREE_PATH, $env:CODEX_WORKTREE_PATH) | Set-Content -Path action-result.txt`
	}
	saved, err := PutProjectEnvironment(ctx, source, ProjectEnvironment{
		ID: "environment.toml", Version: 1, Name: "Development", Setup: EnvironmentScript{},
		Actions: []EnvironmentAction{
			{Name: "Run managed", Icon: "run", Command: actionScript, Platform: platform},
			{Name: "Wrong platform", Icon: "debug", Command: "echo must-not-be-exposed", Platform: otherPlatform},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	projectID := "project_action_contract"
	manager := &Manager{runtimes: map[string]*threadRuntime{
		projectID: {thread: threadstore.Thread{
			ID: projectID, Kind: threadstore.KindProject, CWD: worktree,
			Worktree: &threadstore.WorktreeInfo{
				SourceCWD: source, Path: worktree, EnvironmentID: saved.ID,
			},
		}},
	}}

	_, public, err := projectLaunchers(ctx, manager, nil, projectID)
	if err != nil {
		t.Fatal(err)
	}
	var actionLauncher *configuration.ProjectLauncher
	for index := range public {
		switch public[index].ID {
		case saved.Actions[0].ID:
			actionLauncher = &public[index]
		case saved.Actions[1].ID:
			t.Fatalf("launcher list included action for %s on %s", otherPlatform, platform)
		}
	}
	if actionLauncher == nil {
		t.Fatalf("launcher list omitted current-platform action: %#v", public)
	}
	if actionLauncher.Command != "" || len(actionLauncher.Args) != 0 {
		t.Fatalf("public launcher exposed server-side action command: %#v", *actionLauncher)
	}
	wire, err := json.Marshal(actionLauncher)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), actionScript) || strings.Contains(string(wire), "must-not-be-exposed") {
		t.Fatalf("public launcher JSON exposed an action command: %s", wire)
	}

	project, resolved, err := configuredProjectLauncher(ctx, manager, nil, projectID, saved.Actions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.script != actionScript || resolved.launcher.Command != "" {
		t.Fatalf("server-side launcher resolution = %#v", resolved)
	}
	command, err := projectTerminalCommand(ctx, project, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if command.Dir != worktree {
		t.Fatalf("terminal command cwd = %q, want %q", command.Dir, worktree)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run environment action: %v\n%s", err, output)
	}
	result, err := os.ReadFile(filepath.Join(worktree, "action-result.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(result), "\r\n", "\n")), "\n")
	want := []string{worktree, source, worktree}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("action result = %#v, want %#v", lines, want)
	}
}

func canonicalTestDirectory(t *testing.T, directory string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
