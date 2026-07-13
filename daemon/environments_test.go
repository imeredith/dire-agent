package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectEnvironmentRoundTripRevisionAndDelete(t *testing.T) {
	root := t.TempDir()
	environment := ProjectEnvironment{
		ID: "environment.toml", Version: 1, Name: "Development",
		Setup: EnvironmentScript{
			Script: "echo default",
			Darwin: &EnvironmentPlatformScript{Script: "echo mac"},
			Linux:  &EnvironmentPlatformScript{Script: "echo linux"},
			Win32:  &EnvironmentPlatformScript{Script: "Write-Output windows"},
		},
		Cleanup: &EnvironmentScript{Script: "echo cleanup"},
		Actions: []EnvironmentAction{{Name: "Run", Icon: "run", Command: "npm start"}},
	}
	saved, err := PutProjectEnvironment(context.Background(), root, environment, "")
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Hash == "" || saved.ConfigPath != filepath.Join(canonicalRoot, ".codex", "environments", environment.ID) {
		t.Fatalf("saved metadata = %#v", saved)
	}
	if len(saved.Actions) != 1 || !strings.HasPrefix(saved.Actions[0].ID, "env-") {
		t.Fatalf("saved actions = %#v", saved.Actions)
	}
	data, err := os.ReadFile(saved.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, daemonField := range []string{"config_path", "hash =", "id ="} {
		if strings.Contains(string(data), daemonField) {
			t.Fatalf("TOML contains daemon metadata %q:\n%s", daemonField, data)
		}
	}
	listed, err := ListProjectEnvironments(context.Background(), root)
	if err != nil || len(listed) != 1 || listed[0].Hash != saved.Hash {
		t.Fatalf("listed = %#v, err = %v", listed, err)
	}
	environment.Name = "Changed"
	if _, err := PutProjectEnvironment(context.Background(), root, environment, ""); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("empty-hash overwrite error = %v", err)
	}
	if _, err := PutProjectEnvironment(context.Background(), root, environment, "stale"); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale update error = %v", err)
	}
	updated, err := PutProjectEnvironment(context.Background(), root, environment, saved.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Changed" || updated.Hash == saved.Hash {
		t.Fatalf("updated environment = %#v", updated)
	}
	if err := DeleteProjectEnvironment(context.Background(), root, updated.ID, saved.Hash); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale delete error = %v", err)
	}
	if err := DeleteProjectEnvironment(context.Background(), root, updated.ID, updated.Hash); err != nil {
		t.Fatal(err)
	}
	if listed, err := ListProjectEnvironments(context.Background(), root); err != nil || len(listed) != 0 {
		t.Fatalf("after delete = %#v, err = %v", listed, err)
	}
}

func TestProjectEnvironmentRejectsUnknownFieldsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, ".codex", "environments")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "invalid.toml"), []byte("version=1\nname='x'\nunknown=true\n[setup]\nscript=''\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ListProjectEnvironments(context.Background(), root); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := PutProjectEnvironment(context.Background(), root, ProjectEnvironment{
		ID: "../escape.toml", Version: 1, Name: "Escape", Setup: EnvironmentScript{},
	}, ""); err == nil {
		t.Fatal("path-traversing environment id was accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(outside, []byte("version=1\nname='outside'\n[setup]\nscript=''\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "linked.toml")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectEnvironment(context.Background(), root, "linked.toml"); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink load error = %v", err)
	}
	if err := DeleteProjectEnvironment(context.Background(), root, "linked.toml", ""); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink delete error = %v", err)
	}
}

func TestEnvironmentPlatformScriptsOverrideDefault(t *testing.T) {
	script := EnvironmentScript{
		Script: "default",
		Darwin: &EnvironmentPlatformScript{Script: "mac"},
		Linux:  &EnvironmentPlatformScript{Script: "linux"},
		Win32:  &EnvironmentPlatformScript{Script: "windows"},
	}
	for platform, want := range map[string]string{"darwin": "mac", "linux": "linux", "win32": "windows"} {
		if got := environmentScriptForPlatform(script, platform); got != want {
			t.Fatalf("platform %s script = %q, want %q", platform, got, want)
		}
	}
	script.Linux.Script = ""
	if got := environmentScriptForPlatform(script, "linux"); got != "default" {
		t.Fatalf("empty override = %q, want fallback", got)
	}
}

func TestProjectEnvironmentRejectsUnsupportedVersionAndOversizedDocument(t *testing.T) {
	root := t.TempDir()
	if _, err := PutProjectEnvironment(context.Background(), root, ProjectEnvironment{
		ID: "future.toml", Version: 2, Name: "Future", Setup: EnvironmentScript{},
	}, ""); err == nil || !strings.Contains(err.Error(), "version must be 1") {
		t.Fatalf("future version error = %v", err)
	}
	actions := make([]EnvironmentAction, maxEnvironmentActions)
	for index := range actions {
		actions[index] = EnvironmentAction{Name: "Large", Command: strings.Repeat("x", 20<<10)}
	}
	if _, err := PutProjectEnvironment(context.Background(), root, ProjectEnvironment{
		ID: "large.toml", Version: 1, Name: "Large", Setup: EnvironmentScript{}, Actions: actions,
	}, ""); err == nil || !strings.Contains(err.Error(), "encoded project environment is too large") {
		t.Fatalf("oversized document error = %v", err)
	}
}
