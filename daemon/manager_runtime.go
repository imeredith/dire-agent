package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/skills"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (m *Manager) getRuntime(ctx context.Context, id string) (*threadRuntime, error) {
	m.mu.Lock()
	if runtime := m.runtimes[id]; runtime != nil {
		m.mu.Unlock()
		return runtime, nil
	}
	m.mu.Unlock()

	db, err := m.config.Store.Open(ctx, id)
	if err != nil {
		return nil, err
	}
	thread, err := db.Thread(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	state, stateErr := db.LoadState(ctx)
	if stateErr != nil && !errors.Is(stateErr, sql.ErrNoRows) {
		db.Close()
		return nil, stateErr
	}
	var statePointer *threadstore.State
	if stateErr == nil {
		statePointer = &state
	}
	runtime, err := m.runtimeFromDB(ctx, db, thread, statePointer)
	if err != nil {
		db.Close()
		return nil, err
	}
	m.mu.Lock()
	if existing := m.runtimes[id]; existing != nil {
		m.mu.Unlock()
		db.Close()
		return existing, nil
	}
	m.runtimes[id] = runtime
	m.mu.Unlock()
	return runtime, nil
}

func (m *Manager) runtimeFromDB(ctx context.Context, db *threadstore.ThreadDB, thread threadstore.Thread, state *threadstore.State) (*threadRuntime, error) {
	if thread.Kind == "" {
		thread.Kind = threadstore.KindProject
	}
	if thread.Usage.ContextWindow == 0 {
		if model, ok := m.modelInfo(thread.Model); ok {
			thread.Usage.ContextWindow = model.ContextWindow
		}
	}
	snapshot, err := m.resolveCapabilities(ctx, thread)
	if err != nil {
		return nil, err
	}
	options := agent.SessionOptions{
		Model: thread.Model, WorkingDirectory: thread.CWD,
		Instructions: sessionInstructions(thread, snapshot.Instructions),
	}
	var session agent.Session
	if state == nil {
		if err := m.validateActiveModel(thread.Model); err != nil {
			return nil, err
		}
		session, err = m.config.Provider.OpenSession(ctx, options)
	} else {
		session, err = m.config.Provider.OpenSessionFromState(ctx, options, agent.SessionState{
			ID: state.SessionID, Provider: state.Provider, Data: state.Data,
		})
	}
	if err != nil {
		return nil, err
	}
	if err := m.validateActiveModel(thread.Model); err != nil {
		return nil, err
	}
	stepSession, ok := session.(agent.StepSession)
	if !ok {
		return nil, errors.New("daemon: provider session does not support agentic steps")
	}
	stateful, ok := session.(agent.StatefulSession)
	if !ok {
		return nil, errors.New("daemon: provider session does not support persistence")
	}
	if thread.Status == "running" {
		thread.Status = "idle"
		thread.UpdatedAt = time.Now().UTC()
		if _, err := db.UpdateThread(ctx, func(stored *threadstore.Thread) error {
			*stored = thread
			return nil
		}); err != nil {
			return nil, fmt.Errorf("daemon: recover interrupted thread: %w", err)
		}
	}
	return &threadRuntime{
		manager: m, db: db, session: stepSession, stateful: stateful,
		tools: snapshot.Tools, thread: thread,
		capabilityInstructions: snapshot.Instructions,
		capabilities:           append([]capability.Descriptor(nil), snapshot.Descriptors...),
		skills:                 append([]skills.Skill(nil), snapshot.Skills...),
		skillDiagnostics:       append([]skills.Diagnostic(nil), snapshot.Diagnostics...),
		preparePrompt:          snapshot.PreparePrompt,
		hooks:                  snapshot.Hooks,
		commands:               snapshot.Commands,
	}, nil
}

func (m *Manager) DeleteThread(ctx context.Context, id string) error {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	if runtime.running || runtime.finishing {
		runtime.mu.Unlock()
		return errors.New("daemon: cannot delete a running conversation")
	}
	runtime.mu.Unlock()
	m.teamMu.Lock()
	defer m.teamMu.Unlock()
	resources, err := m.config.Store.List(ctx)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		if resource.ParentID == id {
			return errors.New("daemon: cannot delete a conversation while it has child agents")
		}
	}
	thread := runtime.snapshotThread()
	m.mu.Lock()
	delete(m.runtimes, id)
	m.mu.Unlock()
	_ = runtime.db.Close()
	if err := m.config.Store.Delete(id); err != nil {
		return err
	}
	delete(m.teamMailboxes, id)
	m.notifyTeamLocked(teamRootID(thread))
	return nil
}

func (m *Manager) DeleteProject(ctx context.Context, id string) error {
	if _, err := m.Project(ctx, id); err != nil {
		return err
	}
	return m.DeleteThread(ctx, id)
}

func (m *Manager) DeleteChat(ctx context.Context, id string) error {
	if _, err := m.Chat(ctx, id); err != nil {
		return err
	}
	return m.DeleteThread(ctx, id)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	runtimes := make([]*threadRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtimes = append(runtimes, runtime)
	}
	m.runtimes = make(map[string]*threadRuntime)
	m.mu.Unlock()
	for _, runtime := range runtimes {
		runtime.mu.Lock()
		if runtime.cancel != nil {
			runtime.cancel()
		}
		runtime.mu.Unlock()
		runtime.runWG.Wait()
		_ = runtime.db.Close()
	}
	providerErr := m.config.Provider.Close()
	if m.config.Capabilities == nil {
		return providerErr
	}
	return errors.Join(providerErr, m.config.Capabilities.Close())
}

func (r *threadRuntime) saveState(ctx context.Context) error {
	state, err := r.stateful.State()
	if err != nil {
		return err
	}
	return r.db.SaveState(ctx, threadstore.State{Provider: state.Provider, SessionID: state.ID, Data: state.Data})
}

func (r *threadRuntime) persistThread(ctx context.Context) error {
	r.mu.Lock()
	thread := r.thread
	r.mu.Unlock()
	_, err := r.db.UpdateThread(ctx, func(stored *threadstore.Thread) error {
		*stored = thread
		return nil
	})
	return err
}

func (r *threadRuntime) reopenSessionLocked(ctx context.Context) error {
	return r.reopenSessionWithInstructionsLocked(ctx, r.capabilityInstructions)
}

func (r *threadRuntime) reopenSessionWithInstructionsLocked(ctx context.Context, capabilityInstructions string) error {
	state, err := r.stateful.State()
	if err != nil {
		return err
	}
	session, err := r.manager.config.Provider.OpenSessionFromState(ctx, agent.SessionOptions{
		Model: r.thread.Model, WorkingDirectory: r.thread.CWD,
		Instructions: sessionInstructions(r.thread, capabilityInstructions),
	}, state)
	if err != nil {
		return err
	}
	step, ok := session.(agent.StepSession)
	if !ok {
		return errors.New("daemon: restored session lacks step support")
	}
	stateful, ok := session.(agent.StatefulSession)
	if !ok {
		return errors.New("daemon: restored session lacks state support")
	}
	r.session, r.stateful = step, stateful
	return nil
}

func (r *threadRuntime) snapshotThread() threadstore.Thread {
	r.mu.Lock()
	defer r.mu.Unlock()
	thread := r.thread
	thread.Tools = append([]string(nil), thread.Tools...)
	thread.AdditionalFolders = append([]string(nil), thread.AdditionalFolders...)
	if thread.Worktree != nil {
		copy := *thread.Worktree
		thread.Worktree = &copy
	}
	return thread
}
