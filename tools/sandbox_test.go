package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/tools"
)

func TestBashUsesInjectedSandboxExecutableAndProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LD_PRELOAD", filepath.Join(root, "project-owned-library.so"))
	fakeSandbox := writeExecutable(t, `#!/bin/sh
printf 'cwd=<%s>\n' "$PWD"
printf 'ld_preload=<%s>\n' "${LD_PRELOAD-unset}"
printf 'argc=<%s>\n' "$#"
for arg in "$@"; do
  printf 'arg=<%s>\n' "$arg"
done
`)

	const profile = "(version 1) (deny default)"
	var profileWorkspace string
	builtins, err := tools.BuiltinsWithOptions(root, []string{"bash"}, tools.BuiltinOptions{
		SandboxExecutable: fakeSandbox,
		SandboxProfile: func(workspace string, _ []string) (string, error) {
			profileWorkspace = workspace
			return profile, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if profileWorkspace != root {
		t.Fatalf("profile workspace = %q, want %q", profileWorkspace, root)
	}

	const userCommand = `printf '%s' "a command with shell syntax; still one argument"`
	output, err := builtins["bash"].Execute(context.Background(), mustJSON(t, map[string]any{
		"command": userCommand,
	}))
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, output)
	}
	want := strings.Join([]string{
		"cwd=<" + root + ">",
		"ld_preload=<unset>",
		"argc=<5>",
		"arg=<-p>",
		"arg=<" + profile + ">",
		"arg=</bin/sh>",
		"arg=<-c>",
		"arg=<" + userCommand + ">",
		"",
	}, "\n")
	if output != want {
		t.Fatalf("sandbox invocation:\n%s\nwant:\n%s", output, want)
	}
}

func TestBashFailsClosedWhenSandboxExecutableIsUnavailable(t *testing.T) {
	root := t.TempDir()
	profileCalled := false
	_, err := tools.BuiltinsWithOptions(root, []string{"bash"}, tools.BuiltinOptions{
		SandboxExecutable: filepath.Join(t.TempDir(), "missing-sandbox-exec"),
		SandboxProfile: func(string, []string) (string, error) {
			profileCalled = true
			return "(version 1)", nil
		},
	})
	if err == nil {
		t.Fatal("bash was created without an available sandbox executable")
	}
	if !strings.Contains(err.Error(), "refusing to run unsandboxed") {
		t.Fatalf("error = %q", err)
	}
	if profileCalled {
		t.Fatal("profile was built after sandbox validation failed")
	}
}

func TestBashFailsClosedForEmptyOrFailedProfile(t *testing.T) {
	root := t.TempDir()
	fakeSandbox := writeExecutable(t, "#!/bin/sh\nexit 99\n")

	for _, test := range []struct {
		name    string
		builder func(string, []string) (string, error)
		want    string
	}{
		{
			name: "empty",
			builder: func(string, []string) (string, error) {
				return " \n\t", nil
			},
			want: "profile is empty",
		},
		{
			name: "builder error",
			builder: func(string, []string) (string, error) {
				return "", errors.New("profile unavailable")
			},
			want: "profile unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := tools.BuiltinsWithOptions(root, []string{"bash"}, tools.BuiltinOptions{
				SandboxExecutable: fakeSandbox,
				SandboxProfile:    test.builder,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestUnavailableSandboxDoesNotAffectNonBashTools(t *testing.T) {
	root := t.TempDir()
	builtins, err := tools.BuiltinsWithOptions(root, []string{"write", "read"}, tools.BuiltinOptions{
		SandboxExecutable: filepath.Join(root, "missing-sandbox-exec"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtins["write"].Execute(context.Background(), json.RawMessage(`{"path":"ok.txt","content":"confined"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinSandboxExecConfinesWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is a macOS facility")
	}
	root := t.TempDir()
	builtins, err := tools.Builtins(root, []string{"bash"})
	if err != nil {
		t.Skipf("sandbox-exec unavailable: %v", err)
	}

	output, err := builtins["bash"].Execute(context.Background(), json.RawMessage(`{"command":"printf sandbox-ok"}`))
	if err != nil && strings.Contains(output, "sandbox_apply: Operation not permitted") {
		t.Skip("the parent test process forbids applying a nested macOS sandbox")
	}
	if err != nil || output != "sandbox-ok" {
		t.Fatalf("sandbox preflight: output=%q err=%v", output, err)
	}

	if _, err := builtins["bash"].Execute(context.Background(), json.RawMessage(`{"command":"printf inside > inside.txt"}`)); err != nil {
		t.Fatalf("workspace write failed: %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(root, "inside.txt")); err != nil || string(contents) != "inside" {
		t.Fatalf("workspace contents=%q err=%v", contents, err)
	}

	outsideBase, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if pathWithinAny(outsideBase, []string{os.TempDir(), root}) {
		t.Skip("test checkout is within an allowed temporary path")
	}
	outside := filepath.Join(outsideBase, fmt.Sprintf(".dire-agent-sandbox-write-%d", os.Getpid()))
	_ = os.Remove(outside)
	t.Cleanup(func() { _ = os.Remove(outside) })
	command := "printf escaped > " + shellQuote(outside)
	output, err = builtins["bash"].Execute(context.Background(), mustJSON(t, map[string]any{"command": command}))
	if err == nil {
		t.Fatalf("outside write unexpectedly succeeded: %s", output)
	}
	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatalf("sandbox created outside file %q: %v", outside, statErr)
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("create network probe: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	command = fmt.Sprintf("/usr/bin/nc -z -w 1 127.0.0.1 %d", port)
	output, err = builtins["bash"].Execute(context.Background(), mustJSON(t, map[string]any{"command": command}))
	if err == nil {
		t.Fatalf("sandboxed command connected to local TCP listener: %s", output)
	}
}

func TestLinuxBubblewrapConfinesFilesystemAndNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Bubblewrap is a Linux facility")
	}
	root := t.TempDir()
	extra := t.TempDir()
	builtins, err := tools.BuiltinsWithOptions(root, []string{"bash"}, tools.BuiltinOptions{
		AdditionalFolders: []string{extra},
	})
	if err != nil {
		if strings.Contains(err.Error(), "install bubblewrap") {
			t.Skipf("Bubblewrap unavailable: %v", err)
		}
		t.Fatal(err)
	}

	output, err := builtins["bash"].Execute(context.Background(), json.RawMessage(`{"command":"printf sandbox-ok"}`))
	if err != nil && bubblewrapNamespacesUnavailable(output) {
		t.Skipf("host does not permit Bubblewrap namespaces: %s", output)
	}
	if err != nil || output != "sandbox-ok" {
		t.Fatalf("Bubblewrap preflight: output=%q err=%v", output, err)
	}

	if _, err := builtins["bash"].Execute(context.Background(), json.RawMessage(`{"command":"printf inside > inside.txt"}`)); err != nil {
		t.Fatalf("workspace write failed: %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(root, "inside.txt")); err != nil || string(contents) != "inside" {
		t.Fatalf("workspace contents=%q err=%v", contents, err)
	}
	extraFile := filepath.Join(extra, "included.txt")
	command := "printf included > " + shellQuote(extraFile)
	if output, err := builtins["bash"].Execute(context.Background(), mustJSON(t, map[string]any{"command": command})); err != nil {
		t.Fatalf("included-folder write failed: output=%q err=%v", output, err)
	}
	if contents, err := os.ReadFile(extraFile); err != nil || string(contents) != "included" {
		t.Fatalf("included contents=%q err=%v", contents, err)
	}

	secretDirectory := t.TempDir()
	secret := filepath.Join(secretDirectory, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("must stay hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err = builtins["bash"].Execute(context.Background(), mustJSON(t, map[string]any{
		"command": "cat " + shellQuote(secret),
	}))
	if err == nil || strings.Contains(output, "must stay hidden") {
		t.Fatalf("sandbox read unrelated host file: output=%q err=%v", output, err)
	}

	outsideBase, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideBase, fmt.Sprintf(".dire-agent-bwrap-write-%d", os.Getpid()))
	_ = os.Remove(outside)
	t.Cleanup(func() { _ = os.Remove(outside) })
	output, err = builtins["bash"].Execute(context.Background(), mustJSON(t, map[string]any{
		"command": "printf escaped > " + shellQuote(outside),
	}))
	if err == nil && !pathWithinAny(outsideBase, []string{os.TempDir(), "/tmp", "/var/tmp"}) {
		t.Fatalf("outside write unexpectedly succeeded: %s", output)
	}
	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatalf("sandbox created outside file %q: %v", outside, statErr)
	}

	listener, listenErr := net.Listen("tcp4", "127.0.0.1:0")
	if listenErr != nil {
		t.Logf("local listener unavailable; skipping Bubblewrap network probe: %v", listenErr)
		return
	}
	defer listener.Close()
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helperEnvironment := append(tools.SanitizeSandboxEnvironment(os.Environ()), "DIRE_AGENT_BWRAP_DIAL="+listener.Addr().String())
	hostProbe := exec.Command(helper, "-test.run=^TestBubblewrapNetworkDialHelper$")
	hostProbe.Env = helperEnvironment
	if output, err := hostProbe.CombinedOutput(); err != nil {
		t.Fatalf("host network helper preflight: %v\n%s", err, output)
	}

	for _, probe := range []struct {
		name         string
		allowNetwork bool
		wantSuccess  bool
	}{
		{name: "workspace", allowNetwork: true, wantSuccess: true},
		{name: "strict", allowNetwork: false, wantSuccess: false},
	} {
		t.Run("network_"+probe.name, func(t *testing.T) {
			wrapper, args, err := tools.WrapSandboxedProcess(tools.ProcessSandbox{
				Workspace: root, Command: helper,
				Args: []string{"-test.run=^TestBubblewrapNetworkDialHelper$"}, AllowNetwork: probe.allowNetwork,
			})
			if err != nil {
				t.Fatal(err)
			}
			process := exec.Command(wrapper, args...)
			process.Dir = root
			process.Env = helperEnvironment
			output, runErr := process.CombinedOutput()
			if probe.wantSuccess && runErr != nil {
				t.Fatalf("network-enabled sandbox could not connect: %v\n%s", runErr, output)
			}
			if !probe.wantSuccess && (runErr == nil || !strings.Contains(string(output), "network helper dial")) {
				t.Fatalf("strict sandbox network result: err=%v output=%s", runErr, output)
			}
		})
	}
}

func TestBubblewrapNetworkDialHelper(t *testing.T) {
	address := os.Getenv("DIRE_AGENT_BWRAP_DIAL")
	if address == "" {
		return
	}
	connection, err := net.DialTimeout("tcp4", address, time.Second)
	if err != nil {
		t.Fatalf("network helper dial: %v", err)
	}
	_ = connection.Close()
}

func bubblewrapNamespacesUnavailable(output string) bool {
	if !strings.Contains(output, "bwrap:") {
		return false
	}
	for _, message := range []string{
		"Creating new namespace failed",
		"No permissions to create new namespace",
		"Failed to make / slave",
		"setting up uid map",
		"Failed RTM_NEWADDR",
	} {
		if strings.Contains(output, message) {
			return true
		}
	}
	return false
}

func writeExecutable(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-sandbox-exec")
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func pathWithinAny(path string, roots []string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
