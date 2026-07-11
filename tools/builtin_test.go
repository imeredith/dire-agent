package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/tools"
)

func TestReadWriteEditAndPathConfinement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	builtins, err := tools.Builtins(root, []string{"read", "write", "edit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtins["write"].Execute(context.Background(), json.RawMessage(`{"path":"notes/a.txt","content":"hello"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := builtins["edit"].Execute(context.Background(), json.RawMessage(`{"path":"notes/a.txt","old_text":"hello","new_text":"world"}`)); err != nil {
		t.Fatalf("edit: %v", err)
	}
	output, err := builtins["read"].Execute(context.Background(), json.RawMessage(`{"path":"notes/a.txt"}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(output, "world") {
		t.Fatalf("read output = %q", output)
	}
	if _, err := builtins["read"].Execute(context.Background(), json.RawMessage(`{"path":"../outside"}`)); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func TestSymlinkEscapeIsRejected(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	builtins, err := tools.Builtins(root, []string{"read"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtins["read"].Execute(context.Background(), json.RawMessage(`{"path":"link/secret"}`)); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}
