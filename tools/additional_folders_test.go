package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dire-kiwi/dire-agent/tools"
)

func TestFileToolsKeepRelativePathsInMainAndAllowAbsoluteIncludedFolders(t *testing.T) {
	main := t.TempDir()
	extra := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(main, "marker.txt"), []byte("main marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "marker.txt"), []byte("extra marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	builtins, err := tools.BuiltinsWithOptions(main, []string{"read", "write", "edit", "ls", "find", "grep"}, tools.BuiltinOptions{
		AdditionalFolders: []string{extra},
	})
	if err != nil {
		t.Fatal(err)
	}

	mainOutput, err := builtins["read"].Execute(context.Background(), json.RawMessage(`{"path":"marker.txt"}`))
	if err != nil || !strings.Contains(mainOutput, "main marker") {
		t.Fatalf("relative read = %q, %v", mainOutput, err)
	}
	extraFile := filepath.Join(extra, "marker.txt")
	extraOutput, err := builtins["read"].Execute(context.Background(), mustJSON(t, map[string]any{"path": extraFile}))
	if err != nil || !strings.Contains(extraOutput, "extra marker") {
		t.Fatalf("included read = %q, %v", extraOutput, err)
	}
	written := filepath.Join(extra, "created.txt")
	output, err := builtins["write"].Execute(context.Background(), mustJSON(t, map[string]any{
		"path": written, "content": "inside included folder",
	}))
	if err != nil || !strings.Contains(output, written) {
		t.Fatalf("included write = %q, %v", output, err)
	}
	if contents, err := os.ReadFile(written); err != nil || string(contents) != "inside included folder" {
		t.Fatalf("included contents = %q, %v", contents, err)
	}

	if _, err := builtins["read"].Execute(context.Background(), mustJSON(t, map[string]any{
		"path": filepath.Join(outside, "secret.txt"),
	})); err == nil || !strings.Contains(err.Error(), "escapes the project sandbox") {
		t.Fatalf("outside read error = %v", err)
	}
	link := filepath.Join(extra, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := builtins["read"].Execute(context.Background(), mustJSON(t, map[string]any{
		"path": filepath.Join(link, "secret.txt"),
	})); err == nil || !strings.Contains(err.Error(), "escapes the project sandbox") {
		t.Fatalf("symlink escape error = %v", err)
	}
}

func TestBashProfileReceivesIncludedWritableFolders(t *testing.T) {
	main := t.TempDir()
	extra := t.TempDir()
	fakeSandbox := writeExecutable(t, "#!/bin/sh\nexit 0\n")
	var got []string
	_, err := tools.BuiltinsWithOptions(main, []string{"bash"}, tools.BuiltinOptions{
		SandboxExecutable: fakeSandbox,
		AdditionalFolders: []string{extra},
		SandboxProfile: func(_ string, folders []string) (string, error) {
			got = append([]string(nil), folders...)
			return "(version 1) (deny default)", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(extra)
	if len(got) != 1 || got[0] != canonical {
		t.Fatalf("additional folders = %q, want %q", got, canonical)
	}
}
