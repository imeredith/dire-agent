package capability

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/extensions"
	"github.com/dire-kiwi/dire-agent/skills"
)

type ExtensionDiscoverer func(context.Context, extensions.DiscoverOptions) (extensions.Catalog, error)

type ExtensionSourceOptions struct {
	Connector   extensions.Connector
	Limits      extensions.Limits
	PluginRoots []string
	Discover    ExtensionDiscoverer
}

// ExtensionSource catalogs Codex/Pi manifests and runs only explicitly
// configured, trusted local adapters. Remote source kinds remain metadata.
type ExtensionSource struct {
	connector        extensions.Connector
	limits           extensions.Limits
	pluginRoots      []string
	discover         ExtensionDiscoverer
	sandboxProcesses bool
	lifetime         context.Context
	cancel           context.CancelFunc

	mu      sync.Mutex
	clients map[string]extensionClientRecord
	closed  bool
}

type extensionClientRecord struct {
	fingerprint string
	client      *extensions.Client
}

var _ Source = (*ExtensionSource)(nil)

func NewExtensionSource(options ExtensionSourceOptions) *ExtensionSource {
	discover := options.Discover
	if discover == nil {
		discover = extensions.Discover
	}
	lifetime, cancel := context.WithCancel(context.Background())
	return &ExtensionSource{
		connector: options.Connector, limits: options.Limits,
		pluginRoots: append([]string(nil), options.PluginRoots...),
		discover:    discover, lifetime: lifetime, cancel: cancel,
		sandboxProcesses: options.Connector == nil,
		clients:          make(map[string]extensionClientRecord),
	}
}

func (s *ExtensionSource) Name() string { return "extensions" }

func (s *ExtensionSource) Resolve(ctx context.Context, scope Scope, settings configuration.Settings) (Fragment, error) {
	if err := ctx.Err(); err != nil {
		return Fragment{}, err
	}
	if s.isClosed() {
		return Fragment{}, errors.New("capability: extension source is closed")
	}
	local, remote := configuredExtensionSources(settings.Extensions)
	fragment := Fragment{Tools: make(map[string]agentloop.Tool)}
	if s.sandboxProcesses {
		var sandboxDescriptors []Descriptor
		local, sandboxDescriptors = sandboxExtensionSources(local, scope, settings.Tools.Sandbox)
		fragment.Descriptors = append(fragment.Descriptors, sandboxDescriptors...)
	}
	for _, entry := range remote {
		fragment.Descriptors = append(fragment.Descriptors, remoteExtensionDescriptor(entry.name, entry.source))
	}
	catalog, err := s.discover(ctx, extensions.DiscoverOptions{
		Sources: local, PluginRoots: append([]string(nil), s.pluginRoots...),
	})
	if err != nil {
		if ctx.Err() != nil {
			return Fragment{}, ctx.Err()
		}
		fragment.Descriptors = append(fragment.Descriptors, safeExtensionDiagnostic("discovery", "discovery-failed", "error", 0))
		s.reconcile(scopeKey(scope), nil)
		return fragment, nil
	}
	found := make(map[string]bool)
	keep := make(map[string]string)
	for _, extension := range catalog.Extensions {
		found[extension.ID] = true
		s.resolveExtension(ctx, scope, extension, &fragment, keep)
	}
	for _, entry := range local {
		id := extensionID(entry.ID)
		if !found[id] {
			fragment.Descriptors = append(fragment.Descriptors, Descriptor{
				Name: "extension:" + id, Source: "extension", Description: "Local extension could not be catalogued.",
				Enabled: false, Status: "invalid",
			})
		}
	}
	for index, diagnostic := range catalog.Diagnostics {
		fragment.Descriptors = append(fragment.Descriptors,
			safeExtensionDiagnostic(diagnostic.ExtensionID, diagnostic.Code, string(diagnostic.Severity), index))
	}
	s.reconcile(scopeKey(scope), keep)
	sort.Slice(fragment.Descriptors, func(i, j int) bool { return fragment.Descriptors[i].Name < fragment.Descriptors[j].Name })
	return fragment, nil
}

func (s *ExtensionSource) resolveExtension(ctx context.Context, scope Scope, extension extensions.Extension, fragment *Fragment, keep map[string]string) {
	descriptor := extensionDescriptor(extension)
	fragment.Descriptors = append(fragment.Descriptors, descriptor)
	if extension.Enabled && extension.Trust == extensions.TrustTrusted &&
		(extension.State == extensions.StateRunnable || extension.State == extensions.StateCatalogued) {
		for _, path := range extension.SkillRoots {
			fragment.PluginSkillRoots = append(fragment.PluginSkillRoots, skills.PluginRoot{Name: extension.ID, Path: path})
		}
	}
	if extension.State != extensions.StateRunnable {
		return
	}
	slot := scopeKey(scope) + "\x00" + extension.ID
	fingerprint := extensionFingerprint(extension)
	keep[slot] = fingerprint
	client, err := s.clientFor(ctx, slot, fingerprint, extension.LaunchConfig())
	if err != nil {
		descriptorIndex := len(fragment.Descriptors) - 1
		fragment.Descriptors[descriptorIndex].Enabled = false
		fragment.Descriptors[descriptorIndex].Status = "error"
		fragment.Descriptors = append(fragment.Descriptors,
			safeExtensionDiagnostic(extension.ID, "adapter-unavailable", "error", 0))
		return
	}
	addExtensionContributions(client, extension.ID, fragment)
	for name, tool := range client.AgentTools() {
		if _, exists := fragment.Tools[name]; exists {
			fragment.Descriptors = append(fragment.Descriptors,
				safeExtensionDiagnostic(extension.ID, "duplicate-tool", "error", 0))
			continue
		}
		fragment.Tools[name] = tool
		fragment.Descriptors = append(fragment.Descriptors, descriptorForExtensionTool(tool, extension.ID))
	}
}

func (s *ExtensionSource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	clients := make([]*extensions.Client, 0, len(s.clients))
	for _, record := range s.clients {
		clients = append(clients, record.client)
	}
	s.clients = map[string]extensionClientRecord{}
	s.mu.Unlock()
	var result error
	for _, client := range clients {
		result = errors.Join(result, closeExtensionClient(client))
	}
	s.cancel()
	return result
}

func (s *ExtensionSource) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
