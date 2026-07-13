package capability

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/extensions"
)

func TestTrustedExtensionProcessIsSandboxWrapped(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no process sandbox is available on this platform")
	}
	root := t.TempDir()
	sources, diagnostics := sandboxExtensionSources([]extensions.Source{{
		ID: "safe", Location: root, Enabled: true, Trust: extensions.TrustTrusted,
		Command: "/usr/bin/printf", Args: []string{"one;argument"},
	}}, Scope{Kind: "project", CWD: root}, configuration.SandboxStrict)
	if runtime.GOOS == "linux" && len(diagnostics) == 1 && strings.Contains(diagnostics[0].Description, "install bubblewrap") {
		t.Skip(diagnostics[0].Description)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	if !sources[0].Sandboxed {
		t.Fatal("wrapped extension was not marked for environment sanitization")
	}
	switch runtime.GOOS {
	case "darwin":
		if sources[0].Command != "/usr/bin/sandbox-exec" || len(sources[0].Args) < 4 {
			t.Fatalf("source = %+v", sources[0])
		}
		if !strings.Contains(sources[0].Args[1], "(deny network*)") || sources[0].Args[len(sources[0].Args)-1] != "one;argument" {
			t.Fatalf("sandbox args = %#v", sources[0].Args)
		}
	case "linux":
		bubblewrap, err := filepath.EvalSymlinks("/usr/bin/bwrap")
		if err != nil {
			t.Fatal(err)
		}
		if sources[0].Command != bubblewrap || !containsStrings(sources[0].Args, "--unshare-net", "--chdir", root, "--", "/usr/bin/printf", "one;argument") {
			t.Fatalf("source = %+v", sources[0])
		}
	}
}

func TestExtensionSandboxWorkingDirectoryMatchesDiscoveryRoot(t *testing.T) {
	root := t.TempDir()
	manifestDirectory := filepath.Join(root, ".codex-plugin")
	if err := os.MkdirAll(manifestDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(manifestDirectory, "plugin.json")
	if err := os.WriteFile(manifest, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := extensionSandboxWorkingDirectory(manifest); got != root {
		t.Fatalf("plugin working directory = %q, want %q", got, root)
	}
	if got := extensionSandboxWorkingDirectory(root); got != root {
		t.Fatalf("directory working directory = %q, want %q", got, root)
	}
}

func containsStrings(values []string, wanted ...string) bool {
	position := 0
	for _, value := range values {
		if position < len(wanted) && value == wanted[position] {
			position++
		}
	}
	return position == len(wanted)
}
