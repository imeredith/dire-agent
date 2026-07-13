package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWrapSandboxedProcessUsesArgvAndWorkspaceProfile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is a macOS facility")
	}
	root := t.TempDir()
	command, args, err := WrapSandboxedProcess(ProcessSandbox{
		Workspace: root, Command: "/usr/bin/printf", Args: []string{"a;still-one-arg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if command != "/usr/bin/sandbox-exec" || len(args) != 4 || args[2] != "/usr/bin/printf" || args[3] != "a;still-one-arg" {
		t.Fatalf("wrapper = %q %#v", command, args)
	}
	canonical, _ := filepath.EvalSymlinks(root)
	if !strings.Contains(args[1], "(deny network*)") || !strings.Contains(args[1], canonical) {
		t.Fatalf("profile = %s", args[1])
	}
}

func TestProcessSandboxWorkspaceModeAllowsNetwork(t *testing.T) {
	root := t.TempDir()
	profile, err := sandboxProfile(root, []string{"/usr/bin/printf"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(profile, "(deny network*)") {
		t.Fatalf("workspace-mode profile denied network: %s", profile)
	}
}

func TestDarwinProcessSandboxCanReadDifferentWorkingDirectory(t *testing.T) {
	workspace := t.TempDir()
	workingDirectory := t.TempDir()
	executable := filepath.Join(t.TempDir(), "sandbox-exec")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(t.TempDir(), "command")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, args, err := wrapSandboxedProcessForPlatform("darwin", executable, ProcessSandbox{
		Workspace: workspace, WorkingDirectory: workingDirectory, Command: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorkingDirectory, _ := filepath.EvalSymlinks(workingDirectory)
	if !strings.Contains(args[1], canonicalWorkingDirectory) {
		t.Fatalf("profile cannot read working directory %q: %s", canonicalWorkingDirectory, args[1])
	}
}
