package capability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/configuration"
)

type recordingSettingsStore struct{ id string }

func (s *recordingSettingsStore) RuntimeSettings(_ context.Context, id string) (configuration.Settings, bool, error) {
	s.id = id
	return configuration.DefaultConfig(os.TempDir()).Global, false, nil
}

func TestRegistryCombinesBuiltinsAndTrustedSkills(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: review\ndescription: Review changes carefully.\n---\n# Review\nRun tests.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := configuration.DefaultConfig(root).Global
	settings.Skills.Trust = configuration.TrustTrusted
	registry := NewRegistry(RegistryConfig{Defaults: settings})
	snapshot, err := registry.Resolve(context.Background(), Scope{
		ConversationID: "project_1", Kind: "project", CWD: root, Builtins: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Tools["read"] == nil || snapshot.Tools["skill"] == nil {
		t.Fatalf("tools = %v", snapshot.Tools)
	}
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].Name != "review" {
		t.Fatalf("skills = %+v", snapshot.Skills)
	}
	if snapshot.Instructions == "" {
		t.Fatal("trusted skill catalog was not added to instructions")
	}
	expanded, err := snapshot.PreparePrompt(context.Background(), "/skill:review changed files")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expanded, "Run tests.") || !strings.Contains(expanded, "Arguments: changed files") {
		t.Fatalf("expanded prompt = %q", expanded)
	}
}

func TestStandaloneChatGetsNoFolderBuiltins(t *testing.T) {
	settings := configuration.DefaultConfig(t.TempDir()).Global
	registry := NewRegistry(RegistryConfig{Defaults: settings})
	snapshot, err := registry.Resolve(context.Background(), Scope{
		ConversationID: "chat_1", Kind: "chat", Builtins: []string{"read", "bash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Tools["read"] != nil || snapshot.Tools["bash"] != nil {
		t.Fatalf("chat received local tools: %v", snapshot.Tools)
	}
}

func TestRegistryUsesSeparateSettingsIdentity(t *testing.T) {
	store := &recordingSettingsStore{}
	registry := NewRegistry(RegistryConfig{Settings: store})
	root := t.TempDir()
	if _, err := registry.Resolve(context.Background(), Scope{
		ConversationID: "agent_child", SettingsID: "project_root",
		Kind: "project", CWD: root,
	}); err != nil {
		t.Fatal(err)
	}
	if store.id != "project_root" {
		t.Fatalf("settings id = %q, want project_root", store.id)
	}
}

func TestPromptTrustCatalogsButDoesNotExposeSkillTool(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "one")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: one\ndescription: One.\n---\nBody"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := configuration.DefaultConfig(root).Global
	registry := NewRegistry(RegistryConfig{Defaults: settings})
	snapshot, err := registry.Resolve(context.Background(), Scope{Kind: "project", CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Tools["skill"] != nil || len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
