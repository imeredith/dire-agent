// Package daemon manages persistent agent conversations and their asynchronous runs.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/skills"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

type ManagerConfig struct {
	Store           *threadstore.Store
	Provider        agent.StatefulProvider
	DefaultProvider string
	DefaultModel    string
	DefaultCWD      string
	DefaultTools    []string
	DefaultThinking string
	MaxAgentSteps   int
	AvailableModels []ModelInfo
	Settings        *configuration.Store
	Capabilities    capability.Resolver
	WorktreeRoot    string
}

type Manager struct {
	config   ManagerConfig
	mu       sync.Mutex
	runtimes map[string]*threadRuntime

	subMu       sync.Mutex
	subscribers map[string]map[uint64]chan Event
	nextSubID   atomic.Uint64

	teamMu        sync.Mutex
	teamSignals   map[string]chan struct{}
	teamMailboxes map[string][]agentteam.Message

	worktreeMu sync.Mutex
}

type threadRuntime struct {
	manager                *Manager
	db                     *threadstore.ThreadDB
	session                agent.StepSession
	stateful               agent.StatefulSession
	tools                  map[string]agentloop.Tool
	capabilityInstructions string
	capabilities           []capability.Descriptor
	skills                 []skills.Skill
	skillDiagnostics       []skills.Diagnostic
	preparePrompt          func(context.Context, string) (string, error)
	hooks                  agentloop.Hooks
	commands               map[string]capability.Command

	mu        sync.Mutex
	thread    threadstore.Thread
	running   bool
	finishing bool
	steering  []string
	followUps []string
	cancel    context.CancelFunc
	runWG     sync.WaitGroup
}

func NewManager(config ManagerConfig) (*Manager, error) {
	if config.Store == nil {
		return nil, errors.New("daemon: project store is required")
	}
	if config.Provider == nil {
		return nil, errors.New("daemon: stateful provider is required")
	}
	if config.DefaultModel == "" {
		config.DefaultModel = "gpt-5.6"
	}
	if config.DefaultProvider == "" {
		config.DefaultProvider = "codex"
	}
	if config.DefaultThinking == "" {
		config.DefaultThinking = "medium"
	}
	if config.DefaultCWD == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		config.DefaultCWD = cwd
	}
	if len(config.DefaultTools) == 0 {
		config.DefaultTools = []string{"read", "grep", "find", "ls"}
	}
	if config.MaxAgentSteps <= 0 {
		config.MaxAgentSteps = 32
	}
	if strings.TrimSpace(config.WorktreeRoot) == "" {
		config.WorktreeRoot = filepath.Join(filepath.Dir(config.Store.Directory()), "worktrees")
	}
	worktreeRoot, err := filepath.Abs(config.WorktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("daemon: resolve worktree root: %w", err)
	}
	worktreeRoot = filepath.Clean(worktreeRoot)
	storeDirectory := filepath.Clean(config.Store.Directory())
	if invalidWorktreeRoot(worktreeRoot, storeDirectory) {
		return nil, errors.New("daemon: worktree root must not be the filesystem root or overlap the project store")
	}
	rootInfo, statErr := os.Lstat(worktreeRoot)
	switch {
	case statErr == nil:
		if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
			return nil, errors.New("daemon: worktree root must be a non-symlink directory")
		}
		if rootInfo.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("daemon: existing worktree root permissions must not grant group or other access")
		}
	case errors.Is(statErr, os.ErrNotExist):
		if err := os.MkdirAll(worktreeRoot, 0o700); err != nil {
			return nil, fmt.Errorf("daemon: create worktree root: %w", err)
		}
	default:
		return nil, fmt.Errorf("daemon: inspect worktree root: %w", statErr)
	}
	worktreeRoot, err = filepath.EvalSymlinks(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("daemon: canonicalize worktree root: %w", err)
	}
	info, err := os.Lstat(worktreeRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("daemon: worktree root must be a directory")
	}
	canonicalStore := storeDirectory
	if resolvedStore, resolveErr := filepath.EvalSymlinks(storeDirectory); resolveErr == nil {
		canonicalStore = resolvedStore
	}
	if invalidWorktreeRoot(worktreeRoot, canonicalStore) {
		return nil, errors.New("daemon: worktree root must not be the filesystem root or overlap the project store")
	}
	config.WorktreeRoot = worktreeRoot
	if len(config.AvailableModels) == 0 {
		if config.DefaultProvider == "codex" {
			config.AvailableModels = defaultModels()
		} else {
			config.AvailableModels = []ModelInfo{{Provider: config.DefaultProvider, ID: config.DefaultModel}}
		}
	}
	config.AvailableModels = normalizeModels(config.AvailableModels, config.DefaultProvider, config.DefaultModel)
	return &Manager{
		config:        config,
		runtimes:      make(map[string]*threadRuntime),
		subscribers:   make(map[string]map[uint64]chan Event),
		teamSignals:   make(map[string]chan struct{}),
		teamMailboxes: make(map[string][]agentteam.Message),
	}, nil
}

func invalidWorktreeRoot(root, store string) bool {
	return filepath.Dir(root) == root || root == store || pathWithin(root, store) || pathWithin(store, root)
}
