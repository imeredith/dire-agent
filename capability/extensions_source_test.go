package capability

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imeredith/dire-agent/configuration"
)

func TestExtensionSourceDiscoversToolsSkillsAndReusesPerScope(t *testing.T) {
	root := extensionFixture(t, "demo")
	connector := &fakeExtensionConnector{}
	source := NewExtensionSource(ExtensionSourceOptions{Connector: connector})
	settings := extensionSettings(root, "adapter", "do-not-expose")
	scope := Scope{ConversationID: "project-1", Kind: "project", CWD: root}

	fragment, err := source.Resolve(context.Background(), scope, settings)
	if err != nil {
		t.Fatal(err)
	}
	tool := fragment.Tools["ext__demo__echo"]
	if tool == nil {
		t.Fatalf("tools = %v", fragment.Tools)
	}
	output, err := tool.Execute(context.Background(), json.RawMessage(`{"value":"hello"}`))
	if err != nil || output != "hello" {
		t.Fatalf("tool output = %q, %v", output, err)
	}
	if len(fragment.PluginSkillRoots) != 1 || fragment.PluginSkillRoots[0].Name != "demo" {
		t.Fatalf("skill roots = %+v", fragment.PluginSkillRoots)
	}
	assertNoDescriptorText(t, fragment.Descriptors, "do-not-expose")
	if connector.connectCount() != 1 || connector.lastSpec().Env["TOKEN"] != "do-not-expose" {
		t.Fatalf("connector state = count %d, spec %+v", connector.connectCount(), connector.lastSpec())
	}
	if _, err := source.Resolve(context.Background(), scope, settings); err != nil {
		t.Fatal(err)
	}
	if connector.connectCount() != 1 {
		t.Fatalf("same scope/config launched %d clients", connector.connectCount())
	}
	other := scope
	other.ConversationID = "project-2"
	if _, err := source.Resolve(context.Background(), other, settings); err != nil {
		t.Fatal(err)
	}
	if connector.connectCount() != 2 {
		t.Fatalf("second scope launched %d clients", connector.connectCount())
	}

	settings.Extensions.Sources["demo"] = disabledExtension(settings.Extensions.Sources["demo"])
	fragment, err = source.Resolve(context.Background(), scope, settings)
	if err != nil || len(fragment.Tools) != 0 {
		t.Fatalf("disabled resolve = %+v, %v", fragment, err)
	}
	if connector.closedCount() != 1 {
		t.Fatalf("disabled source closed %d clients", connector.closedCount())
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if connector.closedCount() != 2 {
		t.Fatalf("close count = %d", connector.closedCount())
	}
}

func TestExtensionSourceIsolatesLaunchFailureAndNeverInstallsRemote(t *testing.T) {
	goodRoot := extensionFixture(t, "good")
	badRoot := extensionFixture(t, "bad")
	connector := &fakeExtensionConnector{}
	source := NewExtensionSource(ExtensionSourceOptions{Connector: connector})
	settings := configuration.DefaultConfig(t.TempDir()).Global
	settings.Extensions.Sources = map[string]configuration.ExtensionSource{
		"good": extensionSettings(goodRoot, "adapter", "good-secret").Extensions.Sources["demo"],
		"bad":  extensionSettings(badRoot, "broken", "failure-secret").Extensions.Sources["demo"],
		"remote": {
			Kind: configuration.ExtensionGit, Location: "https://example.test/plugin.git",
			Trust: configuration.TrustTrusted, Enabled: true,
		},
	}
	fragment, err := source.Resolve(context.Background(), Scope{ConversationID: "one"}, settings)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.Tools["ext__good__echo"] == nil || fragment.Tools["ext__bad__echo"] != nil {
		t.Fatalf("tools = %v", fragment.Tools)
	}
	if connector.connectCount() != 2 {
		t.Fatalf("remote source triggered a connection or local source was skipped: %d", connector.connectCount())
	}
	if !hasExtensionStatus(fragment.Descriptors, "extension:bad", "error") ||
		!hasExtensionStatus(fragment.Descriptors, "extension:remote", "install_unsupported") {
		t.Fatalf("descriptors = %+v", fragment.Descriptors)
	}
	assertNoDescriptorText(t, fragment.Descriptors, "failure-secret")
	_ = source.Close()
}

func TestExtensionSourceConcurrentResolveIsRaceSafe(t *testing.T) {
	root := extensionFixture(t, "demo")
	connector := &fakeExtensionConnector{}
	source := NewExtensionSource(ExtensionSourceOptions{Connector: connector})
	settings := extensionSettings(root, "adapter", "secret")
	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			fragment, err := source.Resolve(context.Background(), Scope{ConversationID: "shared"}, settings)
			if err != nil || fragment.Tools["ext__demo__echo"] == nil {
				t.Errorf("resolve = %+v, %v", fragment, err)
			}
		}()
	}
	wait.Wait()
	if connector.connectCount() != 1 {
		t.Fatalf("connections = %d", connector.connectCount())
	}
	_ = source.Close()
}

func extensionSettings(root, command, secret string) configuration.Settings {
	settings := configuration.DefaultConfig(root).Global
	settings.Extensions.Sources = map[string]configuration.ExtensionSource{"demo": {
		Kind: configuration.ExtensionLocal, Location: root, Trust: configuration.TrustTrusted,
		Enabled: true, Command: command, Env: map[string]string{"TOKEN": secret}, SecretEnv: []string{"TOKEN"},
	}}
	return settings
}

func disabledExtension(source configuration.ExtensionSource) configuration.ExtensionSource {
	source.Enabled = false
	return source
}

func extensionFixture(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".codex-plugin"))
	mustMkdir(t, filepath.Join(root, "skills", "review"))
	mustWrite(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"`+name+`","skills":"skills"}`)
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review.\n---\nBody")
	return root
}

func hasExtensionStatus(descriptors []Descriptor, name, status string) bool {
	for _, descriptor := range descriptors {
		if descriptor.Name == name && descriptor.Status == status {
			return true
		}
	}
	return false
}

func assertNoDescriptorText(t *testing.T, descriptors []Descriptor, forbidden string) {
	t.Helper()
	contents, _ := json.Marshal(descriptors)
	if strings.Contains(string(contents), forbidden) {
		t.Fatalf("descriptors exposed %q: %s", forbidden, contents)
	}
}
