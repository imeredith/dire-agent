package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestMCPServerOverrideWirePreservesFalseAndNullInheritance(t *testing.T) {
	var disabled daemon.Command
	if err := json.Unmarshal([]byte(`{"type":"set_mcp_server_enabled","mcp_server":"docs","enabled":false}`), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled == nil || *disabled.Enabled {
		t.Fatalf("false wire value = %#v", disabled.Enabled)
	}
	encoded, err := json.Marshal(disabled)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"enabled":false`)) {
		t.Fatalf("encoded command = %q", encoded)
	}

	var inherited daemon.Command
	if err := json.Unmarshal([]byte(`{"type":"set_mcp_server_enabled","mcp_server":"docs","enabled":null}`), &inherited); err != nil {
		t.Fatal(err)
	}
	if inherited.Enabled != nil {
		t.Fatalf("null inheritance value = %#v", inherited.Enabled)
	}
}

func TestMCPServerOverridesLayerProjectAndChildThreadChoices(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.MCP.Servers["docs"] = configuration.MCPServer{
		Transport: configuration.MCPStdio, Command: "docs-mcp",
		Approval: configuration.ApprovalNever, Enabled: true,
	}
	settings, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := &scopeRecordingResolver{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
		Settings: settings, Capabilities: resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	project, err = manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{
		MCPServer: &daemon.MCPServerUpdate{Name: "docs", Enabled: &disabled},
	})
	if err != nil {
		t.Fatal(err)
	}
	if enabled, exists := project.MCPServerOverrides["docs"]; !exists || enabled {
		t.Fatalf("project overrides = %#v", project.MCPServerOverrides)
	}

	childID := "agent_mcp_override_test"
	childDB, err := store.Create(ctx, threadstore.Thread{
		ID: childID, Kind: threadstore.KindProject, ParentID: project.ID, RootID: project.ID,
		AgentName: "child", AgentProfile: "general", AgentTools: []string{"mcp__docs__lookup"}, Depth: 1,
		Model: "fake-model", CWD: root, ThinkingLevel: "medium",
		SteeringMode: "one-at-a-time", FollowUpMode: "one-at-a-time", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := childDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CapabilityState(ctx, childID); err != nil {
		t.Fatal(err)
	}
	if enabled, exists := resolver.last(childID).MCPServerOverrides["docs"]; !exists || enabled {
		t.Fatalf("child did not inherit project disable: %#v", resolver.last(childID).MCPServerOverrides)
	}

	enabled := true
	noGrantID := "agent_mcp_without_grant"
	noGrantDB, err := store.Create(ctx, threadstore.Thread{
		ID: noGrantID, Kind: threadstore.KindProject, ParentID: project.ID, RootID: project.ID,
		AgentName: "restricted", AgentProfile: "general", Depth: 1,
		Model: "fake-model", CWD: root, ThinkingLevel: "medium",
		SteeringMode: "one-at-a-time", FollowUpMode: "one-at-a-time", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := noGrantDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateSettings(ctx, noGrantID, daemon.SettingsUpdate{
		MCPServer: &daemon.MCPServerUpdate{Name: "docs", Enabled: &enabled},
	}); err == nil {
		t.Fatal("child enabled an MCP server outside its persisted spawn grant")
	}

	child, err := manager.UpdateSettings(ctx, childID, daemon.SettingsUpdate{
		MCPServer: &daemon.MCPServerUpdate{Name: "docs", Enabled: &enabled},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !child.MCPServerOverrides["docs"] || !resolver.last(childID).MCPServerOverrides["docs"] {
		t.Fatalf("child enable was not applied: metadata=%#v scope=%#v", child.MCPServerOverrides, resolver.last(childID).MCPServerOverrides)
	}

	child, err = manager.UpdateSettings(ctx, childID, daemon.SettingsUpdate{
		MCPServer: &daemon.MCPServerUpdate{Name: "docs", Enabled: nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := child.MCPServerOverrides["docs"]; exists {
		t.Fatalf("clearing override left local choice: %#v", child.MCPServerOverrides)
	}
	if enabled, exists := resolver.last(childID).MCPServerOverrides["docs"]; !exists || enabled {
		t.Fatalf("cleared child did not resume project inheritance: %#v", resolver.last(childID).MCPServerOverrides)
	}

	if _, err := manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{
		MCPServer: &daemon.MCPServerUpdate{Name: "docs", Enabled: nil},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CapabilityState(ctx, childID); err != nil {
		t.Fatal(err)
	}
	if _, exists := resolver.last(childID).MCPServerOverrides["docs"]; exists {
		t.Fatalf("child did not return to global inheritance: %#v", resolver.last(childID).MCPServerOverrides)
	}

	if _, err := manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{
		MCPServer: &daemon.MCPServerUpdate{Name: "missing", Enabled: &enabled},
	}); err == nil {
		t.Fatal("unknown MCP server override succeeded")
	}
}

func TestRootMCPDisableSerializesWithChildCapabilityRefresh(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.MCP.Servers["docs"] = configuration.MCPServer{
		Transport: configuration.MCPStdio, Command: "docs-mcp",
		Approval: configuration.ApprovalNever, Enabled: true,
	}
	settings, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := newBlockingScopeResolver()
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
		Settings: settings, Capabilities: resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	childID := "agent_concurrent_mcp_refresh"
	childDB, err := store.Create(ctx, threadstore.Thread{
		ID: childID, Kind: threadstore.KindProject, ParentID: project.ID, RootID: project.ID,
		AgentName: "child", AgentProfile: "general", Depth: 1,
		Model: "fake-model", CWD: root, ThinkingLevel: "medium",
		SteeringMode: "one-at-a-time", FollowUpMode: "one-at-a-time", Tools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := childDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CapabilityState(ctx, childID); err != nil {
		t.Fatal(err)
	}

	entered, release := resolver.blockNext(childID)
	refreshDone := make(chan error, 1)
	go func() {
		_, refreshErr := manager.CapabilityState(ctx, childID)
		refreshDone <- refreshErr
	}()
	waitChannel(t, entered, "child refresh did not enter the resolver")

	disabled := false
	updateStarted := make(chan struct{})
	updateDone := make(chan error, 1)
	go func() {
		close(updateStarted)
		_, updateErr := manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{
			MCPServer: &daemon.MCPServerUpdate{Name: "docs", Enabled: &disabled},
		})
		updateDone <- updateErr
	}()
	waitChannel(t, updateStarted, "root disable did not start")
	select {
	case err := <-updateDone:
		t.Fatalf("root disable returned before the stale child refresh settled: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	if err := waitError(t, refreshDone, "child refresh did not finish"); err != nil {
		t.Fatal(err)
	}
	if err := waitError(t, updateDone, "root disable did not finish"); err != nil {
		t.Fatal(err)
	}
	if enabled, exists := resolver.last(childID).MCPServerOverrides["docs"]; !exists || enabled {
		t.Fatalf("stale child capabilities won after root disable: %#v", resolver.last(childID).MCPServerOverrides)
	}
}

func TestGlobalCapabilityRefreshSkipsRunningConversation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := &scopeRecordingResolver{}
	provider := newWaitingProvider()
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root, DefaultModel: "fake-model",
		Capabilities: resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Prompt(ctx, project.ID, "hold this run", ""); err != nil {
		t.Fatal(err)
	}
	waitChannel(t, provider.session.started, "provider run did not start")
	before := resolver.callCount()
	if err := manager.RefreshCapabilities(ctx); err != nil {
		t.Fatal(err)
	}
	if after := resolver.callCount(); after != before {
		t.Fatalf("running conversation was resolved during global refresh: before=%d after=%d", before, after)
	}
	close(provider.session.release)
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
}

type scopeRecordingResolver struct {
	mu     sync.Mutex
	scopes map[string]capability.Scope
	calls  int
}

func (r *scopeRecordingResolver) Resolve(_ context.Context, scope capability.Scope) (capability.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scopes == nil {
		r.scopes = make(map[string]capability.Scope)
	}
	r.calls++
	scope.MCPServerOverrides = cloneTestBoolMap(scope.MCPServerOverrides)
	r.scopes[scope.ConversationID] = scope
	return capability.Snapshot{Tools: map[string]agentloop.Tool{}, Commands: map[string]capability.Command{}}, nil
}

func (r *scopeRecordingResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *scopeRecordingResolver) Close() error { return nil }

func (r *scopeRecordingResolver) last(id string) capability.Scope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.scopes[id]
}

func cloneTestBoolMap(input map[string]bool) map[string]bool {
	if input == nil {
		return nil
	}
	result := make(map[string]bool, len(input))
	for name, enabled := range input {
		result[name] = enabled
	}
	return result
}

var _ capability.Resolver = (*scopeRecordingResolver)(nil)

type blockingScopeResolver struct {
	scopeRecordingResolver
	blockMu sync.Mutex
	blockID string
	entered chan struct{}
	release chan struct{}
}

func newBlockingScopeResolver() *blockingScopeResolver { return &blockingScopeResolver{} }

func (r *blockingScopeResolver) blockNext(id string) (<-chan struct{}, chan struct{}) {
	r.blockMu.Lock()
	defer r.blockMu.Unlock()
	r.blockID = id
	r.entered = make(chan struct{})
	r.release = make(chan struct{})
	return r.entered, r.release
}

func (r *blockingScopeResolver) Resolve(ctx context.Context, scope capability.Scope) (capability.Snapshot, error) {
	r.blockMu.Lock()
	shouldBlock := r.blockID == scope.ConversationID
	entered, release := r.entered, r.release
	if shouldBlock {
		r.blockID = ""
	}
	r.blockMu.Unlock()
	if shouldBlock {
		close(entered)
		select {
		case <-ctx.Done():
			return capability.Snapshot{}, ctx.Err()
		case <-release:
		}
	}
	return r.scopeRecordingResolver.Resolve(ctx, scope)
}

var _ capability.Resolver = (*blockingScopeResolver)(nil)

type waitingProvider struct{ session *waitingSession }

func newWaitingProvider() *waitingProvider {
	return &waitingProvider{session: &waitingSession{started: make(chan struct{}), release: make(chan struct{})}}
}

func (p *waitingProvider) OpenSession(context.Context, agent.SessionOptions) (agent.Session, error) {
	return p.session, nil
}

func (p *waitingProvider) OpenSessionFromState(context.Context, agent.SessionOptions, agent.SessionState) (agent.Session, error) {
	return p.session, nil
}

func (*waitingProvider) Close() error { return nil }

type waitingSession struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
}

func (*waitingSession) ID() string { return "waiting-session" }

func (s *waitingSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *waitingSession) Step(ctx context.Context, _ agent.StepRequest) (agent.StepResult, error) {
	s.startedOnce.Do(func() { close(s.started) })
	select {
	case <-ctx.Done():
		return agent.StepResult{}, ctx.Err()
	case <-s.release:
		return agent.StepResult{Result: agent.Result{Text: "done", SessionID: s.ID()}}, nil
	}
}

func (s *waitingSession) State() (agent.SessionState, error) {
	return agent.SessionState{ID: s.ID(), Provider: "waiting", Data: json.RawMessage(`{}`)}, nil
}

func waitChannel(t *testing.T, channel <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func waitError(t *testing.T, channel <-chan error, message string) error {
	t.Helper()
	select {
	case err := <-channel:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal(message)
		return nil
	}
}

var _ agent.StatefulProvider = (*waitingProvider)(nil)
var _ agent.StepSession = (*waitingSession)(nil)
var _ agent.StatefulSession = (*waitingSession)(nil)
