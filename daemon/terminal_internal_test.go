package daemon

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/configuration"
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
