package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPathPrefersDireAgentAndFallsBackToGoAgent(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	current := filepath.Join(home, ".dire-agent", "config.json")
	legacy := filepath.Join(home, ".goagent", "config.json")

	if got := DefaultPath(home); got != current {
		t.Fatalf("empty home config path = %q, want %q", got, current)
	}
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DefaultPath(home); got != legacy {
		t.Fatalf("legacy config path = %q, want %q", got, legacy)
	}
	if err := os.MkdirAll(filepath.Dir(current), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DefaultPath(home); got != current {
		t.Fatalf("current config path = %q, want %q", got, current)
	}
}

func TestDefaultConfigIsValidAndIndependent(t *testing.T) {
	home := t.TempDir()
	first := DefaultConfig(home)
	second := DefaultConfig(home)
	if err := Validate(first); err != nil {
		t.Fatalf("Validate(DefaultConfig): %v", err)
	}
	if first.Global.Model.ID != "gpt-5.6" {
		t.Fatalf("model = %q", first.Global.Model.ID)
	}
	if first.Global.Tools.Sandbox != SandboxStrict {
		t.Fatalf("sandbox = %q", first.Global.Tools.Sandbox)
	}
	if first.Global.Desktop.CodexHome != filepath.Join(home, ".codex") {
		t.Fatalf("Codex home = %q", first.Global.Desktop.CodexHome)
	}
	first.Global.Tools.Enabled[0] = "changed"
	first.Global.Launchers[2].Args[0] = "changed"
	first.Global.MCP.Servers["new"] = MCPServer{}
	if second.Global.Tools.Enabled[0] == "changed" || second.Global.Launchers[2].Args[0] == "changed" || len(second.Global.MCP.Servers) != 0 {
		t.Fatal("defaults share mutable state")
	}
	if got := []string{second.Global.Launchers[0].ID, second.Global.Launchers[1].ID, second.Global.Launchers[2].ID}; got[0] != "shell" || got[1] != "lazygit" || got[2] != "nvim" {
		t.Fatalf("default launchers = %v", got)
	}
}

func TestEffectiveDeepMergeAndCopy(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	provider := "openai"
	maxPending := 2
	emptyTools := []string{}
	syncSkills := false
	always := ApprovalAlways
	disabled := false
	config.Global.MCP.Servers["global"] = validStdioServer()
	config.Global.Extensions.Sources["builtin"] = ExtensionSource{
		Kind: ExtensionRegistry, Location: "example/builtin", Trust: TrustTrusted, Enabled: true,
	}
	config.Projects["project-a"] = ProjectOverride{
		Folder: filepath.Join(t.TempDir(), "project"),
		Settings: SettingsPatch{
			Model:  &ModelPatch{Provider: &provider},
			Tools:  &ToolPatch{Enabled: &emptyTools},
			Queues: &QueuePatch{MaxPending: &maxPending},
			MCP: &MCPPatch{Servers: map[string]MCPServerPatch{
				"global": {Approval: &always},
				"local":  patchForServer(validHTTPServer()),
			}},
			Extensions: &ExtensionPatch{Sources: map[string]ExtensionSourcePatch{
				"builtin": {Enabled: &disabled},
			}},
			Desktop: &DesktopPatch{SyncSkills: &syncSkills},
		},
	}
	if err := Validate(config); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	effective, found := config.Effective("project-a")
	if !found {
		t.Fatal("project not found")
	}
	if effective.Model.Provider != provider || effective.Model.ID != "gpt-5.6" {
		t.Fatalf("model was not deeply merged: %+v", effective.Model)
	}
	if effective.Queues.MaxPending != 2 || effective.Queues.SteeringMode != QueueOneAtATime {
		t.Fatalf("queues were not deeply merged: %+v", effective.Queues)
	}
	if len(effective.Tools.Enabled) != 0 {
		t.Fatalf("explicit empty tools ignored: %v", effective.Tools.Enabled)
	}
	if effective.Desktop.SyncSkills {
		t.Fatal("explicit false override ignored")
	}
	if len(effective.MCP.Servers) != 2 {
		t.Fatalf("MCP map was not merged: %v", effective.MCP.Servers)
	}
	if effective.MCP.Servers["global"].Command != "mcp-server" || effective.MCP.Servers["global"].Approval != ApprovalAlways {
		t.Fatalf("MCP server was not deeply merged: %+v", effective.MCP.Servers["global"])
	}
	if _, ok := effective.Extensions.Sources["builtin"]; !ok {
		t.Fatal("global extension was lost")
	}
	if effective.Extensions.Sources["builtin"].Enabled {
		t.Fatal("extension source was not deeply merged")
	}

	effective.MCP.Servers["global"] = MCPServer{}
	effective.Skills.Roots[0] = "mutated"
	if config.Global.MCP.Servers["global"].Command == "" || config.Global.Skills.Roots[0] == "mutated" {
		t.Fatal("effective settings alias source configuration")
	}
}

func TestLauncherPatchReplacesAndPreservesExplicitEmptyList(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	empty := []ProjectLauncher{}
	config.Projects["no-launchers"] = ProjectOverride{
		Folder:   filepath.Join(t.TempDir(), "no-launchers"),
		Settings: SettingsPatch{Launchers: &empty},
	}
	cloned, err := cloneConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	effective, found := cloned.Effective("no-launchers")
	if !found || effective.Launchers == nil || len(effective.Launchers) != 0 {
		t.Fatalf("explicit empty launchers were not preserved: found=%v launchers=%#v", found, effective.Launchers)
	}
	legacy := config.Global
	legacy.Launchers = nil
	if got := ResolveProjectLaunchers(legacy.Launchers); len(got) != 3 || got[0].ID != "shell" {
		t.Fatalf("legacy launcher fallback = %#v", got)
	}
}

func TestEffectivePreservesExplicitEmptySubagentTools(t *testing.T) {
	config := DefaultConfig(t.TempDir())
	config.Global.Subagents.Profiles["no-tools"] = AgentProfile{
		Description: "Agent with no callable project tools.",
		Tools:       []string{},
	}
	effective, _ := config.Effective("missing")
	profile := effective.Subagents.Profiles["no-tools"]
	if profile.Tools == nil {
		t.Fatal("explicit empty profile tools became nil/inherit")
	}
	effective.Subagents.Profiles["no-tools"] = AgentProfile{Description: "mutated"}
	if config.Global.Subagents.Profiles["no-tools"].Description == "mutated" {
		t.Fatal("effective subagent profiles alias source configuration")
	}
}

func validStdioServer() MCPServer {
	return MCPServer{
		Transport: MCPStdio, Command: "mcp-server", Approval: ApprovalOnRequest, Enabled: true,
	}
}

func validHTTPServer() MCPServer {
	return MCPServer{
		Transport: MCPStreamableHTTP, URL: "https://example.test/mcp", Approval: ApprovalNever, Enabled: true,
	}
}

func patchForServer(server MCPServer) MCPServerPatch {
	return MCPServerPatch{
		Transport:     &server.Transport,
		Command:       &server.Command,
		Args:          &server.Args,
		InheritEnv:    &server.InheritEnv,
		URL:           &server.URL,
		Env:           &server.Env,
		SecretEnv:     &server.SecretEnv,
		Headers:       &server.Headers,
		SecretHeaders: &server.SecretHeaders,
		EnabledTools:  &server.EnabledTools,
		Approval:      &server.Approval,
		ToolApprovals: &server.ToolApprovals,
		Enabled:       &server.Enabled,
	}
}
