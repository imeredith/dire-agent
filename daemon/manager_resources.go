package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/skills"
	"github.com/dire-kiwi/dire-agent/threadstore"
	"github.com/dire-kiwi/dire-agent/tools"
)

func (m *Manager) CreateThread(ctx context.Context, options CreateThreadOptions) (threadstore.Thread, error) {
	category, err := normalizeProjectCategory(options.Category)
	if err != nil {
		return threadstore.Thread{}, err
	}
	id, err := newResourceID(threadstore.KindProject)
	if err != nil {
		return threadstore.Thread{}, err
	}
	settingsID := id
	providedCWD := strings.TrimSpace(options.CWD) != ""
	if options.CWD == "" {
		options.CWD = m.config.DefaultCWD
	}
	if options.Worktree != nil && strings.TrimSpace(options.Worktree.SourceProjectID) != "" {
		source, sourceErr := m.Project(ctx, strings.TrimSpace(options.Worktree.SourceProjectID))
		if sourceErr != nil || source.IsSubagent() {
			return threadstore.Thread{}, errors.New("daemon: worktree source must be a top-level project")
		}
		sourceFolder := source.CWD
		if source.Worktree != nil && source.Worktree.SourceCWD != "" {
			sourceFolder = source.Worktree.SourceCWD
		}
		sourceFolder, err = canonicalProjectFolder(sourceFolder)
		if err != nil {
			return threadstore.Thread{}, err
		}
		if providedCWD {
			provided, resolveErr := canonicalProjectFolder(options.CWD)
			if resolveErr != nil {
				return threadstore.Thread{}, resolveErr
			}
			if provided != sourceFolder {
				return threadstore.Thread{}, errors.New("daemon: worktree folder does not match source_project_id")
			}
		}
		options.CWD = sourceFolder
		settingsID = firstNonEmpty(source.SettingsID, source.ID)
	}
	options.CWD, err = canonicalProjectFolder(options.CWD)
	if err != nil {
		return threadstore.Thread{}, err
	}
	settings, err := m.runtimeSettings(ctx, settingsID)
	if err != nil {
		return threadstore.Thread{}, err
	}
	if options.Model == "" {
		options.Model = firstNonEmpty(settings.Model.ID, m.config.DefaultModel)
	}
	var worktree *threadstore.WorktreeInfo
	var rollbackWorktree func() error
	worktreePublished := false
	if options.Worktree != nil {
		request := *options.Worktree
		request.EnvironmentID = strings.TrimSpace(request.EnvironmentID)
		options.CWD, worktree, rollbackWorktree, err = m.createManagedWorktree(ctx, id, options.CWD, request)
		if err != nil {
			return threadstore.Thread{}, err
		}
		defer func() {
			if !worktreePublished {
				_ = rollbackWorktree()
			}
		}()
	}
	options.AdditionalFolders, err = canonicalAdditionalFolders(options.CWD, options.AdditionalFolders)
	if err != nil {
		if rollbackWorktree != nil {
			err = errors.Join(err, rollbackWorktree())
		}
		return threadstore.Thread{}, err
	}
	if options.ThinkingLevel == "" {
		options.ThinkingLevel = firstNonEmpty(string(settings.Thinking.Level), m.config.DefaultThinking)
	}
	if len(options.Tools) == 0 {
		options.Tools = append([]string(nil), settings.Tools.Enabled...)
		if len(options.Tools) == 0 {
			options.Tools = append([]string(nil), m.config.DefaultTools...)
		}
	}
	if _, err := tools.BuiltinsWithOptions(options.CWD, options.Tools, tools.BuiltinOptions{
		AdditionalFolders: options.AdditionalFolders,
	}); err != nil {
		if rollbackWorktree != nil {
			err = errors.Join(err, rollbackWorktree())
		}
		return threadstore.Thread{}, err
	}
	thread := threadstore.Thread{
		ID: id, Kind: threadstore.KindProject, Name: options.Name, Category: category,
		Model: options.Model, CWD: options.CWD, Worktree: worktree,
		AdditionalFolders: append([]string(nil), options.AdditionalFolders...),
		Instructions:      options.Instructions,
		ThinkingLevel:     options.ThinkingLevel,
		SteeringMode:      firstNonEmpty(string(settings.Queues.SteeringMode), "one-at-a-time"),
		FollowUpMode:      firstNonEmpty(string(settings.Queues.FollowUpMode), "one-at-a-time"),
		Tools:             append([]string(nil), options.Tools...), Status: "idle",
	}
	if settingsID != id {
		thread.SettingsID = settingsID
	}
	if model, ok := m.modelInfo(thread.Model); ok {
		thread.Usage.ContextWindow = model.ContextWindow
	}
	created, err := m.createResource(ctx, thread, "project_created", "thread_created")
	if err != nil {
		if rollbackWorktree != nil {
			err = errors.Join(err, rollbackWorktree())
		}
		return threadstore.Thread{}, err
	}
	worktreePublished = true
	return created, nil
}

func normalizeProjectCategory(category string) (string, error) {
	category = strings.TrimSpace(category)
	if len([]rune(category)) > 80 {
		return "", errors.New("daemon: project category must be 80 characters or fewer")
	}
	for _, value := range category {
		if value < ' ' || value == 0x7f {
			return "", errors.New("daemon: project category must not contain control characters")
		}
	}
	return category, nil
}

func (m *Manager) createResource(ctx context.Context, resource threadstore.Thread, eventTypes ...string) (threadstore.Thread, error) {
	db, err := m.config.Store.Create(ctx, resource)
	if err != nil {
		return threadstore.Thread{}, err
	}
	stored, err := db.Thread(ctx)
	if err != nil {
		db.Close()
		_ = m.config.Store.Delete(resource.ID)
		return threadstore.Thread{}, err
	}
	runtime, err := m.runtimeFromDB(ctx, db, stored, nil)
	if err != nil {
		db.Close()
		_ = m.config.Store.Delete(resource.ID)
		return threadstore.Thread{}, err
	}
	if err := runtime.saveState(ctx); err != nil {
		db.Close()
		_ = m.config.Store.Delete(resource.ID)
		return threadstore.Thread{}, err
	}
	m.mu.Lock()
	m.runtimes[resource.ID] = runtime
	m.mu.Unlock()
	for _, eventType := range eventTypes {
		m.emit(ctx, runtime, eventType, resource)
	}
	return runtime.snapshotThread(), nil
}

// CreateProject creates a persistent project scoped to a canonical folder.
func (m *Manager) CreateProject(ctx context.Context, options CreateProjectOptions) (threadstore.Thread, error) {
	return m.CreateThread(ctx, options)
}

// CreateChat creates a persistent conversation without a project folder. It
// intentionally starts without local file or shell tools.
func (m *Manager) CreateChat(ctx context.Context, options CreateChatOptions) (threadstore.Chat, error) {
	id, err := newResourceID(threadstore.KindChat)
	if err != nil {
		return threadstore.Chat{}, err
	}
	settings, err := m.runtimeSettings(ctx, id)
	if err != nil {
		return threadstore.Chat{}, err
	}
	if options.Model == "" {
		options.Model = firstNonEmpty(settings.StandaloneChat.Model, settings.Model.ID, m.config.DefaultModel)
	}
	if options.ThinkingLevel == "" {
		options.ThinkingLevel = firstNonEmpty(string(settings.StandaloneChat.Thinking), string(settings.Thinking.Level), m.config.DefaultThinking)
	}
	if options.Instructions == "" {
		options.Instructions = settings.StandaloneChat.Instructions
	}
	chat := threadstore.Chat{
		ID: id, Kind: threadstore.KindChat, Name: options.Name, Model: options.Model,
		Instructions: options.Instructions, ThinkingLevel: options.ThinkingLevel,
		SteeringMode: firstNonEmpty(string(settings.Queues.SteeringMode), "one-at-a-time"),
		FollowUpMode: firstNonEmpty(string(settings.Queues.FollowUpMode), "one-at-a-time"),
		Tools:        []string{}, Status: "idle",
	}
	if model, ok := m.modelInfo(chat.Model); ok {
		chat.Usage.ContextWindow = model.ContextWindow
	}
	return m.createResource(ctx, chat, "chat_created", "conversation_created")
}

func (m *Manager) ListThreads(ctx context.Context) ([]threadstore.Thread, error) {
	threads, err := m.config.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	for index := range threads {
		if threads[index].Kind == "" {
			threads[index].Kind = threadstore.KindProject
		}
		if threads[index].Usage.ContextWindow == 0 {
			if model, ok := m.modelInfo(threads[index].Model); ok {
				threads[index].Usage.ContextWindow = model.ContextWindow
			}
		}
	}
	topLevel := threads[:0]
	for _, thread := range threads {
		if !thread.IsSubagent() {
			topLevel = append(topLevel, thread)
		}
	}
	return topLevel, nil
}

func (m *Manager) ListProjects(ctx context.Context) ([]threadstore.Thread, error) {
	resources, err := m.ListThreads(ctx)
	if err != nil {
		return nil, err
	}
	return filterResources(resources, threadstore.KindProject), nil
}

func (m *Manager) ListChats(ctx context.Context) ([]threadstore.Chat, error) {
	resources, err := m.ListThreads(ctx)
	if err != nil {
		return nil, err
	}
	return filterResources(resources, threadstore.KindChat), nil
}

func (m *Manager) ListConversations(ctx context.Context) ([]threadstore.Conversation, error) {
	return m.ListThreads(ctx)
}

func (m *Manager) Thread(ctx context.Context, id string) (threadstore.Thread, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return threadstore.Thread{}, err
	}
	return runtime.snapshotThread(), nil
}

func (m *Manager) Project(ctx context.Context, id string) (threadstore.Thread, error) {
	resource, err := m.Thread(ctx, id)
	if err != nil {
		return threadstore.Project{}, err
	}
	if resource.ResourceKind() != threadstore.KindProject {
		return threadstore.Project{}, errors.New("daemon: conversation is not a project")
	}
	return resource, nil
}

func (m *Manager) Chat(ctx context.Context, id string) (threadstore.Chat, error) {
	resource, err := m.Thread(ctx, id)
	if err != nil {
		return threadstore.Chat{}, err
	}
	if resource.ResourceKind() != threadstore.KindChat {
		return threadstore.Chat{}, errors.New("daemon: conversation is not a standalone chat")
	}
	return resource, nil
}

func (m *Manager) State(ctx context.Context, id string) (RuntimeState, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return RuntimeState{}, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	kind := runtime.thread.ResourceKind()
	var project threadstore.Project
	var chat threadstore.Chat
	if kind == threadstore.KindChat {
		chat = runtime.thread
	} else {
		project = runtime.thread
	}
	return RuntimeState{
		Kind: kind, Conversation: runtime.thread, Project: project, Chat: chat,
		Thread: runtime.thread, Usage: runtime.thread.Usage,
		Capabilities:     append([]capability.Descriptor(nil), runtime.capabilities...),
		Skills:           append([]skills.Skill(nil), runtime.skills...),
		SkillDiagnostics: append([]skills.Diagnostic(nil), runtime.skillDiagnostics...),
		Running:          runtime.running, SteeringQueued: len(runtime.steering), FollowUpsQueued: len(runtime.followUps),
	}, nil
}

func filterResources(resources []threadstore.Thread, kind string) []threadstore.Thread {
	filtered := make([]threadstore.Thread, 0, len(resources))
	for _, resource := range resources {
		if !resource.IsSubagent() && resource.ResourceKind() == kind {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

func (m *Manager) Messages(ctx context.Context, id string, after int64, limit int) ([]threadstore.Message, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	return runtime.db.Messages(ctx, after, limit)
}

func (m *Manager) Events(ctx context.Context, id string, after int64, limit int) ([]threadstore.Event, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	return runtime.db.Events(ctx, after, limit)
}

func newResourceID(kind string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	prefix := "project_"
	if kind == threadstore.KindChat {
		prefix = "chat_"
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func canonicalProjectFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "", errors.New("daemon: project folder is required")
	}
	abs, err := filepath.Abs(folder)
	if err != nil {
		return "", fmt.Errorf("daemon: resolve project folder: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("daemon: resolve project folder %q: %w", folder, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("daemon: inspect project folder %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("daemon: project folder %q is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

const maxAdditionalFolders = 16

func canonicalAdditionalFolders(main string, folders []string) ([]string, error) {
	if len(folders) > maxAdditionalFolders {
		return nil, fmt.Errorf("daemon: a project may include at most %d additional folders", maxAdditionalFolders)
	}
	seen := map[string]bool{main: true}
	normalized := make([]string, 0, len(folders))
	for _, folder := range folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		if !filepath.IsAbs(folder) {
			return nil, fmt.Errorf("daemon: additional sandbox folder must be absolute: %q", folder)
		}
		resolved, err := canonicalProjectFolder(folder)
		if err != nil {
			return nil, errors.New(strings.Replace(err.Error(), "project folder", "additional sandbox folder", 1))
		}
		if filepath.Dir(resolved) == resolved {
			return nil, errors.New("daemon: filesystem root cannot be an additional sandbox folder")
		}
		if seen[resolved] || pathWithin(main, resolved) {
			continue
		}
		seen[resolved] = true
		normalized = append(normalized, resolved)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
