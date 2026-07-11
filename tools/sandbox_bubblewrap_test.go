package tools

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBubblewrapArgsConfineStrictProcessAndPreserveArgv(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "project with spaces")
	extra := t.TempDir()
	readOnly := t.TempDir()
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	command := writeInternalExecutable(t)
	args, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: workspace, WorkingDirectory: workspace,
		Command: command, Args: []string{"one;still-one-argument", "--bind"},
		ExtraReadPaths: []string{readOnly}, AdditionalWritePaths: []string{extra},
		SystemPaths: []string{}, ResolverPaths: []string{},
		TemporaryPaths: []string{"/tmp", "/var/tmp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsArgSequence(args, []string{"--ro-bind", "/", "/"}) {
		t.Fatalf("Bubblewrap exposed the host root: %#v", args)
	}
	canonicalWorkspace, _ := filepath.EvalSymlinks(workspace)
	canonicalExtra, _ := filepath.EvalSymlinks(extra)
	canonicalReadOnly, _ := filepath.EvalSymlinks(readOnly)
	canonicalCommand, _ := filepath.EvalSymlinks(command)
	for _, flag := range []string{
		"--unshare-user", "--unshare-ipc", "--unshare-pid", "--unshare-uts",
		"--unshare-net", "--die-with-parent", "--new-session",
	} {
		if !slices.Contains(args, flag) {
			t.Fatalf("Bubblewrap args do not contain %q: %#v", flag, args)
		}
	}
	for _, sequence := range [][]string{
		{"--cap-drop", "ALL"},
		{"--proc", "/proc"},
		{"--dev", "/dev"},
		{"--perms", "1777", "--tmpfs", "/tmp"},
		{"--perms", "1777", "--tmpfs", "/var/tmp"},
		{"--ro-bind", canonicalReadOnly, readOnly},
		{"--ro-bind", canonicalCommand, command},
		{"--bind", canonicalWorkspace, canonicalWorkspace},
		{"--bind", canonicalExtra, canonicalExtra},
		{"--remount-ro", "/", "--chdir", canonicalWorkspace, "--", command, "one;still-one-argument", "--bind"},
	} {
		if !containsArgSequence(args, sequence) {
			t.Fatalf("Bubblewrap args do not contain sequence %#v: %#v", sequence, args)
		}
	}
}

func TestBubblewrapWorkspaceModeRetainsNetwork(t *testing.T) {
	workspace := t.TempDir()
	command := writeInternalExecutable(t)
	args, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: workspace, Command: command, AllowNetwork: true,
		SystemPaths: []string{}, ResolverPaths: []string{}, TemporaryPaths: []string{"/tmp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "--unshare-net") {
		t.Fatalf("workspace-mode Bubblewrap args isolate network: %#v", args)
	}
}

func TestBubblewrapPrivateWorkspaceDoesNotBindHostDirectory(t *testing.T) {
	workspace := t.TempDir()
	privateTemp := t.TempDir()
	command := writeInternalExecutable(t)
	args, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: workspace, Command: command, PrivateWorkspace: true,
		SystemPaths: []string{}, ResolverPaths: []string{}, TemporaryPaths: []string{privateTemp},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorkspace, _ := filepath.EvalSymlinks(workspace)
	if containsArgSequence(args, []string{"--bind", canonicalWorkspace, canonicalWorkspace}) {
		t.Fatalf("private workspace exposed the host directory: %#v", args)
	}
	if !containsArgSequence(args, []string{"--perms", "0700", "--tmpfs", canonicalWorkspace}) {
		t.Fatalf("private workspace was not created inside the sandbox: %#v", args)
	}
}

func TestBubblewrapPrivateWorkspaceInsidePrivateTemp(t *testing.T) {
	temporaryRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(temporaryRoot, "standalone-workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	command := writeInternalExecutable(t)
	args, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: workspace, Command: command, PrivateWorkspace: true,
		SystemPaths: []string{}, ResolverPaths: []string{}, TemporaryPaths: []string{temporaryRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArgSequence(args, []string{
		"--perms", "1777", "--tmpfs", temporaryRoot,
		"--perms", "0700", "--dir", workspace,
	}) {
		t.Fatalf("nested private workspace mount order is wrong: %#v", args)
	}
	if containsArgSequence(args, []string{"--bind", workspace, workspace}) {
		t.Fatalf("nested private workspace exposed the host directory: %#v", args)
	}
}

func TestBubblewrapRecreatesSystemSymlink(t *testing.T) {
	root := t.TempDir()
	usr := filepath.Join(root, "usr")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(usr, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("usr/bin", bin); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	command := writeInternalExecutable(t)
	args, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: workspace, Command: command,
		SystemPaths: []string{usr, bin}, ResolverPaths: []string{}, TemporaryPaths: []string{"/tmp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArgSequence(args, []string{"--symlink", "usr/bin", bin}) {
		t.Fatalf("Bubblewrap args did not preserve system symlink: %#v", args)
	}
}

func TestBubblewrapMountsTargetOfVisibleSymlink(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := writeInternalExecutable(t)
	command := filepath.Join(workspace, "linked-command")
	if err := os.Symlink(target, command); err != nil {
		t.Fatal(err)
	}
	args, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: workspace, Command: command,
		SystemPaths: []string{}, ResolverPaths: []string{}, TemporaryPaths: []string{"/tmp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget, _ := filepath.EvalSymlinks(target)
	if !containsArgSequence(args, []string{"--ro-bind", resolvedTarget, resolvedTarget}) {
		t.Fatalf("visible symlink target was not mounted: %#v", args)
	}
	if !containsArgSequence(args, []string{"--", command}) {
		t.Fatalf("command symlink path was not preserved: %#v", args)
	}
}

func TestBubblewrapRejectsFilesystemRootWorkspace(t *testing.T) {
	_, err := bubblewrapArgs(bubblewrapConfig{
		Workspace: "/", Command: "/bin/sh",
		SystemPaths: []string{}, ResolverPaths: []string{}, TemporaryPaths: []string{"/tmp"},
	})
	if err == nil || !strings.Contains(err.Error(), "filesystem root") {
		t.Fatalf("error = %v", err)
	}
}

func TestLinuxProcessWrapperUsesInjectedBubblewrap(t *testing.T) {
	workspace := t.TempDir()
	bubblewrap := writeInternalExecutable(t)
	command := writeInternalExecutable(t)
	executable, args, err := wrapSandboxedProcessForPlatform("linux", bubblewrap, ProcessSandbox{
		Workspace: workspace, Command: command, Args: []string{"one;argument"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolvedBubblewrap, _ := filepath.EvalSymlinks(bubblewrap)
	if executable != resolvedBubblewrap {
		t.Fatalf("wrapper executable = %q, want %q", executable, resolvedBubblewrap)
	}
	if !containsArgSequence(args, []string{"--", command, "one;argument"}) {
		t.Fatalf("wrapped command was not kept as direct argv: %#v", args)
	}
}

func TestLinuxProcessWrapperFailsClosedWithoutBubblewrap(t *testing.T) {
	_, _, err := wrapSandboxedProcessForPlatform("linux", filepath.Join(t.TempDir(), "missing-bwrap"), ProcessSandbox{
		Workspace: t.TempDir(), Command: writeInternalExecutable(t),
	})
	if err == nil || !strings.Contains(err.Error(), "install bubblewrap") {
		t.Fatalf("error = %v", err)
	}
}

func TestProcessWrapperRejectsUnsupportedPlatform(t *testing.T) {
	_, _, err := wrapSandboxedProcessForPlatform("plan9", "", ProcessSandbox{
		Workspace: t.TempDir(), Command: writeInternalExecutable(t),
	})
	if err == nil || !strings.Contains(err.Error(), "supported platforms are macOS and Linux") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveRelativeProcessCommandFromWorkingDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	command := filepath.Join(workingDirectory, "adapter")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveProcessCommand("./adapter", workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != command {
		t.Fatalf("resolved command = %q, want %q", resolved, command)
	}
}

func containsArgSequence(args, sequence []string) bool {
	if len(sequence) == 0 {
		return true
	}
	for index := 0; index+len(sequence) <= len(args); index++ {
		if slices.Equal(args[index:index+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func writeInternalExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "executable")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
