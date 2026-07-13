package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrRevisionConflict = errors.New("configuration: revision conflict")
	pathLocks           sync.Map
)

type RevisionConflictError struct {
	Expected uint64
	Actual   uint64
}

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected %d, actual %d", ErrRevisionConflict, e.Expected, e.Actual)
}

func (e *RevisionConflictError) Unwrap() error { return ErrRevisionConflict }

// Store serializes access to one atomically-written JSON configuration file.
// Every value returned by Store has MCP secrets redacted.
type Store struct {
	path     string
	defaults Config
	mu       *sync.Mutex
}

// New uses DefaultConfig for the current user's home directory.
func New(path string) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("configuration: resolve home: %w", err)
	}
	return NewStore(path, DefaultConfig(home))
}

func NewStore(path string, defaults Config) (*Store, error) {
	if path == "" {
		return nil, errors.New("configuration: path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("configuration: resolve path: %w", err)
	}
	if err := Validate(defaults); err != nil {
		return nil, fmt.Errorf("configuration: invalid defaults: %w", err)
	}
	cloned, err := cloneConfig(defaults)
	if err != nil {
		return nil, err
	}
	lock, _ := pathLocks.LoadOrStore(abs, &sync.Mutex{})
	return &Store{path: abs, defaults: cloned, mu: lock.(*sync.Mutex)}, nil
}

func (s *Store) Path() string { return s.path }

// Load creates the file from defaults on first use and returns a public view.
func (s *Store) Load(ctx context.Context) (Config, error) {
	if err := contextErr(ctx); err != nil {
		return Config{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.loadOrCreateLocked(ctx)
	if err != nil {
		return Config{}, err
	}
	return publicConfig(raw), nil
}

// Effective returns redacted effective settings for a project. Global settings
// are returned with found=false when the project id is unknown.
func (s *Store) Effective(ctx context.Context, projectID string) (settings Settings, found bool, err error) {
	settings, found, err = s.runtimeSettings(ctx, projectID)
	if err != nil {
		return Settings{}, false, err
	}
	redactSettings(&settings)
	return settings, found, nil
}

// RuntimeSettings returns the effective settings required by trusted daemon
// internals. Unlike Effective, configured MCP credentials are not redacted.
// Callers must never serialize or expose this value to an untrusted client.
func (s *Store) RuntimeSettings(ctx context.Context, projectID string) (settings Settings, found bool, err error) {
	return s.runtimeSettings(ctx, projectID)
}

func (s *Store) runtimeSettings(ctx context.Context, projectID string) (settings Settings, found bool, err error) {
	if err := contextErr(ctx); err != nil {
		return Settings{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.loadOrCreateLocked(ctx)
	if err != nil {
		return Settings{}, false, err
	}
	settings, found = raw.Effective(projectID)
	return settings, found, nil
}

// Update replaces the document if expectedRevision still matches. Redacted
// placeholders preserve their prior stored values, enabling safe Web UI edits.
func (s *Store) Update(ctx context.Context, expectedRevision uint64, replacement Config) (Config, error) {
	if err := contextErr(ctx); err != nil {
		return Config{}, err
	}
	candidate, err := cloneConfig(replacement)
	if err != nil {
		return Config{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.loadOrCreateLocked(ctx)
	if err != nil {
		return Config{}, err
	}
	if raw.Revision != expectedRevision {
		return Config{}, &RevisionConflictError{Expected: expectedRevision, Actual: raw.Revision}
	}
	candidate.Version = CurrentVersion
	candidate.Revision = raw.Revision + 1
	if candidate.Projects == nil {
		candidate.Projects = map[string]ProjectOverride{}
	}
	restoreRedacted(&candidate, raw)
	if err := Validate(candidate); err != nil {
		return Config{}, err
	}
	if err := s.writeLocked(ctx, candidate); err != nil {
		return Config{}, err
	}
	return publicConfig(candidate), nil
}

// SetProjectSandbox atomically changes one project's process-sandbox override.
// A nil mode removes the override so the project inherits the global default.
func (s *Store) SetProjectSandbox(ctx context.Context, projectID, folder string, mode *SandboxMode) (Config, error) {
	if err := contextErr(ctx); err != nil {
		return Config{}, err
	}
	if !configName.MatchString(projectID) {
		return Config{}, fmt.Errorf("configuration: invalid project id %q", projectID)
	}
	if folder == "" || !filepath.IsAbs(folder) {
		return Config{}, fmt.Errorf("configuration: project %q folder must be absolute", projectID)
	}
	if mode != nil && *mode != SandboxStrict && *mode != SandboxWorkspace && *mode != SandboxOff {
		return Config{}, fmt.Errorf("configuration: invalid sandbox mode %q", *mode)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.loadOrCreateLocked(ctx)
	if err != nil {
		return Config{}, err
	}
	if raw.Projects == nil {
		raw.Projects = make(map[string]ProjectOverride)
	}
	project, exists := raw.Projects[projectID]
	if mode == nil {
		if !exists || project.Settings.Tools == nil || project.Settings.Tools.Sandbox == nil {
			return publicConfig(raw), nil
		}
		tools := *project.Settings.Tools
		tools.Sandbox = nil
		if tools.Enabled == nil && tools.Approval == nil {
			project.Settings.Tools = nil
		} else {
			project.Settings.Tools = &tools
		}
		if emptySettingsPatch(project.Settings) {
			delete(raw.Projects, projectID)
		} else {
			raw.Projects[projectID] = project
		}
	} else {
		if exists && project.Settings.Tools != nil && project.Settings.Tools.Sandbox != nil && *project.Settings.Tools.Sandbox == *mode {
			return publicConfig(raw), nil
		}
		if !exists {
			project = ProjectOverride{Folder: folder}
		}
		tools := ToolPatch{}
		if project.Settings.Tools != nil {
			tools = *project.Settings.Tools
		}
		sandbox := *mode
		tools.Sandbox = &sandbox
		project.Settings.Tools = &tools
		raw.Projects[projectID] = project
	}

	raw.Revision++
	if err := Validate(raw); err != nil {
		return Config{}, err
	}
	if err := s.writeLocked(ctx, raw); err != nil {
		return Config{}, err
	}
	return publicConfig(raw), nil
}

func emptySettingsPatch(patch SettingsPatch) bool {
	return patch.Model == nil && patch.Thinking == nil && patch.Tools == nil && patch.Queues == nil &&
		patch.Skills == nil && patch.MCP == nil && patch.Extensions == nil && patch.Subagents == nil &&
		patch.Launchers == nil && patch.Desktop == nil && patch.StandaloneChat == nil
}

func (s *Store) loadOrCreateLocked(ctx context.Context) (Config, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if err := s.writeLocked(ctx, s.defaults); err != nil {
			return Config{}, err
		}
		return cloneConfig(s.defaults)
	}
	if err != nil {
		return Config{}, fmt.Errorf("configuration: open: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("configuration: decode: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Config{}, err
	}
	if err := Validate(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("configuration: trailing JSON data")
}

func cloneConfig(config Config) (Config, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return Config{}, fmt.Errorf("configuration: clone: %w", err)
	}
	var cloned Config
	if err := json.Unmarshal(data, &cloned); err != nil {
		return Config{}, fmt.Errorf("configuration: clone: %w", err)
	}
	return cloned, nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return errors.New("configuration: nil context")
	}
	return ctx.Err()
}
