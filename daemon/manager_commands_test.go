package daemon_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

type commandResolver struct{ calls int }

func (r *commandResolver) Resolve(context.Context, capability.Scope) (capability.Snapshot, error) {
	return capability.Snapshot{Commands: map[string]capability.Command{
		"ext:demo:echo": {
			Name: "ext:demo:echo", Description: "Echo arguments.", Source: "extension:demo",
			Execute: func(_ context.Context, arguments string) (capability.CommandResult, error) {
				r.calls++
				return capability.CommandResult{Output: arguments}, nil
			},
		},
	}}, nil
}

func (*commandResolver) Close() error { return nil }

func TestCapabilityCommandsAreListedAndExecuted(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := &commandResolver{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, Capabilities: resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	commands, err := manager.CapabilityCommands(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].Name != "ext:demo:echo" || commands[0].Source != "extension:demo" {
		t.Fatalf("commands = %+v", commands)
	}
	result, err := manager.ExecuteCapabilityCommand(ctx, project.ID, "/ext:demo:echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hello" || resolver.calls != 1 {
		t.Fatalf("result = %+v, calls=%d", result, resolver.calls)
	}
	if _, err := manager.ExecuteCapabilityCommand(ctx, project.ID, "ext:missing", ""); err == nil {
		t.Fatal("unknown command was accepted")
	}
}
