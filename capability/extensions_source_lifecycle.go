package capability

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/extensions"
)

func (s *ExtensionSource) clientFor(ctx context.Context, slot, fingerprint string, launch extensions.LaunchConfig) (*extensions.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("capability: extension source is closed")
	}
	if current, ok := s.clients[slot]; ok && current.fingerprint == fingerprint {
		if err := current.client.RefreshTools(ctx); err == nil {
			return current.client, nil
		}
		_ = closeExtensionClient(current.client)
		delete(s.clients, slot)
	} else if ok {
		_ = closeExtensionClient(current.client)
		delete(s.clients, slot)
	}
	client, err := extensions.Open(s.lifetime, launch, extensions.OpenOptions{
		Connector: s.connector, Limits: s.limits,
	})
	if err != nil {
		return nil, err
	}
	s.clients[slot] = extensionClientRecord{fingerprint: fingerprint, client: client}
	return client, nil
}

func (s *ExtensionSource) reconcile(scope string, keep map[string]string) {
	prefix := scope + "\x00"
	var stale []*extensions.Client
	s.mu.Lock()
	for slot, record := range s.clients {
		if !strings.HasPrefix(slot, prefix) {
			continue
		}
		if fingerprint, ok := keep[slot]; ok && fingerprint == record.fingerprint {
			continue
		}
		stale = append(stale, record.client)
		delete(s.clients, slot)
	}
	s.mu.Unlock()
	for _, client := range stale {
		_ = closeExtensionClient(client)
	}
}

func closeExtensionClient(client *extensions.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return client.Close(ctx)
}

func scopeKey(scope Scope) string {
	if scope.ConversationID != "" {
		return scope.ConversationID
	}
	return scope.Kind + "\x00" + scope.CWD
}
