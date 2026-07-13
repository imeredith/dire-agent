package capability

import (
	"runtime"
	"strings"
	"testing"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/extensions"
)

func TestTrustedExtensionProcessIsSandboxWrapped(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is a macOS facility")
	}
	root := t.TempDir()
	sources, diagnostics := sandboxExtensionSources([]extensions.Source{{
		ID: "safe", Location: root, Enabled: true, Trust: extensions.TrustTrusted,
		Command: "/usr/bin/printf", Args: []string{"one;argument"},
	}}, Scope{Kind: "project", CWD: root}, configuration.SandboxStrict)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	if sources[0].Command != "/usr/bin/sandbox-exec" || len(sources[0].Args) < 4 {
		t.Fatalf("source = %+v", sources[0])
	}
	if !strings.Contains(sources[0].Args[1], "(deny network*)") || sources[0].Args[len(sources[0].Args)-1] != "one;argument" {
		t.Fatalf("sandbox args = %#v", sources[0].Args)
	}
}
