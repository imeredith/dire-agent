package daemon

import (
	"context"
	"errors"

	"github.com/imeredith/dire-agent/threadstore"
	"github.com/imeredith/dire-agent/tools"
)

// effectiveSubagentTools reapplies the entire ancestry chain on every
// capability refresh. A child may regain an originally granted tool if a
// parent re-enables it, but it can never exceed its persisted spawn grant.
func (m *Manager) effectiveSubagentTools(ctx context.Context, resource threadstore.Thread) ([]string, error) {
	allowed := make(map[string]bool, len(resource.AgentTools))
	for _, name := range resource.AgentTools {
		allowed[name] = true
	}
	builtins := make(map[string]bool)
	for _, name := range tools.Names() {
		builtins[name] = true
	}
	narrowLocalTools(allowed, resource.Tools, builtins)
	current := resource
	visited := map[string]bool{resource.ID: true}
	for depth := 0; current.ParentID != ""; depth++ {
		if depth >= 128 {
			return nil, errors.New("daemon: subagent ancestry exceeds safety limit")
		}
		if visited[current.ParentID] {
			return nil, errors.New("daemon: cycle in subagent ancestry")
		}
		visited[current.ParentID] = true
		parent, err := m.threadMetadata(ctx, current.ParentID)
		if err != nil {
			return nil, err
		}
		if parent.IsSubagent() {
			parentGrant := make(map[string]bool, len(parent.AgentTools))
			for _, name := range parent.AgentTools {
				parentGrant[name] = true
			}
			for name := range allowed {
				if !parentGrant[name] {
					delete(allowed, name)
				}
			}
		}
		narrowLocalTools(allowed, parent.Tools, builtins)
		current = parent
	}
	result := make([]string, 0, len(resource.AgentTools))
	for _, name := range resource.AgentTools {
		if allowed[name] {
			result = append(result, name)
		}
	}
	return result, nil
}

func narrowLocalTools(allowed map[string]bool, local []string, builtins map[string]bool) {
	localSet := make(map[string]bool, len(local))
	for _, name := range local {
		localSet[name] = true
	}
	for name := range allowed {
		if builtins[name] && !localSet[name] {
			delete(allowed, name)
		}
	}
}

func (m *Manager) threadMetadata(ctx context.Context, id string) (threadstore.Thread, error) {
	m.mu.Lock()
	runtime := m.runtimes[id]
	m.mu.Unlock()
	if runtime != nil {
		return runtime.snapshotThread(), nil
	}
	db, err := m.config.Store.Open(ctx, id)
	if err != nil {
		return threadstore.Thread{}, err
	}
	defer db.Close()
	return db.Thread(ctx)
}
