package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultPaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "tester")
	paths := DefaultPaths(home)
	if want := filepath.Join(home, ".dire-agent", "run", "daemon.json"); paths.RuntimeFile != want {
		t.Fatalf("RuntimeFile = %q, want %q", paths.RuntimeFile, want)
	}
	if want := filepath.Join(home, ".dire-agent", "logs", "daemon.log"); paths.LogFile != want {
		t.Fatalf("LogFile = %q, want %q", paths.LogFile, want)
	}
}

func TestWriteReadAndGuardedRemoveRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".dire-agent", "run", "daemon.json")
	state := testRuntimeState(path, "instance-1234567890")
	if err := WriteRuntime(path, state); err != nil {
		t.Fatalf("WriteRuntime: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("runtime permissions = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("runtime directory permissions = %o, want 700", got)
	}

	got, err := ReadRuntime(path)
	if err != nil {
		t.Fatalf("ReadRuntime: %v", err)
	}
	if got != state {
		t.Fatalf("ReadRuntime = %#v, want %#v", got, state)
	}

	removed, err := RemoveRuntimeIfInstance(path, "different-instance")
	if err != nil {
		t.Fatalf("RemoveRuntimeIfInstance mismatch: %v", err)
	}
	if removed {
		t.Fatal("mismatched instance removed runtime file")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("runtime file missing after guarded mismatch: %v", err)
	}

	removed, err = RemoveRuntimeIfInstance(path, state.InstanceID)
	if err != nil {
		t.Fatalf("RemoveRuntimeIfInstance match: %v", err)
	}
	if !removed {
		t.Fatal("matching instance did not remove runtime file")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime file still exists: %v", err)
	}
}

func TestReadRuntimeRejectsNonPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "daemon.json")
	state := testRuntimeState(path, "instance-1234567890")
	if err := WriteRuntime(path, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRuntime(path); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("ReadRuntime error = %v, want permissions error", err)
	}
}

func TestWriteRuntimeNeverChangesArbitraryParentPermissions(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "daemon.json")
	if err := WriteRuntime(path, testRuntimeState(path, "instance-1234567890")); err == nil || !strings.Contains(err.Error(), "too broad") {
		t.Fatalf("WriteRuntime error = %v, want broad-permissions refusal", err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("parent permissions changed to %o, want 755", got)
	}
}

func TestValidateRuntimeRejectsUnsafeURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	state := testRuntimeState(path, "instance-1234567890")
	state.HTTPURL = "http://192.0.2.1:7331"
	if err := ValidateRuntime(state); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("ValidateRuntime error = %v, want loopback error", err)
	}
	state.HTTPURL = "https://127.0.0.1:7331"
	if err := ValidateRuntime(state); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("ValidateRuntime error = %v, want scheme error", err)
	}
}

func TestRandomValuesAreStrongAndDistinct(t *testing.T) {
	first, err := NewRandomToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 32 || len(second) < 32 || first == second {
		t.Fatalf("tokens are not strong and distinct: lengths %d, %d", len(first), len(second))
	}
	instance, err := NewInstanceID()
	if err != nil {
		t.Fatal(err)
	}
	if len(instance) < 16 {
		t.Fatalf("instance id is too short: %d", len(instance))
	}
}

func testRuntimeState(runtimePath, instanceID string) RuntimeState {
	return RuntimeState{
		Schema:       RuntimeSchema,
		PID:          4242,
		InstanceID:   instanceID,
		ControlToken: strings.Repeat("control-token-", 3),
		Version:      "v1.2.3",
		Executable:   filepath.Join(filepath.Dir(runtimePath), "dire-agent"),
		HTTPURL:      "http://127.0.0.1:7331",
		StartedAt:    time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
	}
}
