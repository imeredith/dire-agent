package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

const (
	defaultEnvironmentID  = "environment.toml"
	maxEnvironmentBytes   = 1 << 20
	maxEnvironmentActions = 64
)

var environmentIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,111}\.toml$`)
var environmentPathLocks sync.Map

// ProjectEnvironment is compatible with Codex project-local environment files
// in .codex/environments. ID, ConfigPath, Hash, and action IDs are daemon-owned
// metadata and are not encoded into TOML.
type ProjectEnvironment struct {
	ID         string              `json:"id" toml:"-"`
	ConfigPath string              `json:"config_path,omitempty" toml:"-"`
	Hash       string              `json:"hash,omitempty" toml:"-"`
	Version    int                 `json:"version" toml:"version"`
	Name       string              `json:"name" toml:"name"`
	Setup      EnvironmentScript   `json:"setup" toml:"setup"`
	Cleanup    *EnvironmentScript  `json:"cleanup,omitempty" toml:"cleanup,omitempty"`
	Actions    []EnvironmentAction `json:"actions,omitempty" toml:"actions,omitempty"`
}

type EnvironmentScript struct {
	Script string                     `json:"script" toml:"script"`
	Darwin *EnvironmentPlatformScript `json:"darwin,omitempty" toml:"darwin,omitempty"`
	Linux  *EnvironmentPlatformScript `json:"linux,omitempty" toml:"linux,omitempty"`
	Win32  *EnvironmentPlatformScript `json:"win32,omitempty" toml:"win32,omitempty"`
}

type EnvironmentPlatformScript struct {
	Script string `json:"script" toml:"script"`
}

type EnvironmentAction struct {
	ID       string `json:"id" toml:"-"`
	Name     string `json:"name" toml:"name"`
	Icon     string `json:"icon,omitempty" toml:"icon,omitempty"`
	Command  string `json:"command" toml:"command"`
	Platform string `json:"platform,omitempty" toml:"platform,omitempty"`
}

func ListProjectEnvironments(ctx context.Context, folder string) ([]ProjectEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := projectEnvironmentDirectory(folder, false)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []ProjectEnvironment{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("daemon: read project environments: %w", err)
	}
	result := make([]ProjectEnvironment, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !environmentIDPattern.MatchString(entry.Name()) {
			continue
		}
		environment, err := loadProjectEnvironmentFile(ctx, directory, entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, environment)
	}
	sortProjectEnvironments(result)
	return result, nil
}

func LoadProjectEnvironment(ctx context.Context, folder, id string) (ProjectEnvironment, error) {
	directory, err := projectEnvironmentDirectory(folder, false)
	if err != nil {
		return ProjectEnvironment{}, err
	}
	return loadProjectEnvironmentFile(ctx, directory, id)
}

func PutProjectEnvironment(ctx context.Context, folder string, environment ProjectEnvironment, expectedHash string) (ProjectEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return ProjectEnvironment{}, err
	}
	if environment.ID == "" {
		environment.ID = defaultEnvironmentID
	}
	if err := validateEnvironmentID(environment.ID); err != nil {
		return ProjectEnvironment{}, err
	}
	if environment.Version == 0 {
		environment.Version = 1
	}
	if err := validateProjectEnvironment(environment); err != nil {
		return ProjectEnvironment{}, err
	}
	directory, err := projectEnvironmentDirectory(folder, true)
	if err != nil {
		return ProjectEnvironment{}, err
	}
	path := filepath.Join(directory, environment.ID)
	lock := environmentPathLock(path)
	lock.Lock()
	defer lock.Unlock()
	if expectedHash == "" {
		if _, statErr := os.Lstat(path); statErr == nil {
			return ProjectEnvironment{}, errors.New("daemon: environment revision conflict: file already exists")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return ProjectEnvironment{}, fmt.Errorf("daemon: inspect environment for creation: %w", statErr)
		}
	} else {
		current, readErr := readRegularEnvironmentFile(path, environment.ID)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return ProjectEnvironment{}, errors.New("daemon: environment revision conflict: file no longer exists")
			}
			return ProjectEnvironment{}, fmt.Errorf("daemon: read environment for update: %w", readErr)
		}
		if contentHash(current) != expectedHash {
			return ProjectEnvironment{}, errors.New("daemon: environment revision conflict")
		}
	}
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(environment); err != nil {
		return ProjectEnvironment{}, fmt.Errorf("daemon: encode project environment: %w", err)
	}
	if encoded.Len() > maxEnvironmentBytes {
		return ProjectEnvironment{}, errors.New("daemon: encoded project environment is too large")
	}
	temporary, err := os.CreateTemp(directory, ".environment-*.tmp")
	if err != nil {
		return ProjectEnvironment{}, fmt.Errorf("daemon: create environment temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o644); err != nil {
		cleanup()
		return ProjectEnvironment{}, fmt.Errorf("daemon: set environment file permissions: %w", err)
	}
	if _, err := temporary.Write(encoded.Bytes()); err != nil {
		cleanup()
		return ProjectEnvironment{}, fmt.Errorf("daemon: write project environment: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return ProjectEnvironment{}, fmt.Errorf("daemon: sync project environment: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return ProjectEnvironment{}, fmt.Errorf("daemon: close project environment: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(temporaryPath)
		return ProjectEnvironment{}, err
	}
	if expectedHash == "" {
		if _, statErr := os.Lstat(path); statErr == nil {
			_ = os.Remove(temporaryPath)
			return ProjectEnvironment{}, errors.New("daemon: environment revision conflict: file already exists")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			_ = os.Remove(temporaryPath)
			return ProjectEnvironment{}, fmt.Errorf("daemon: inspect environment for creation: %w", statErr)
		}
	} else {
		current, readErr := readRegularEnvironmentFile(path, environment.ID)
		if readErr != nil || contentHash(current) != expectedHash {
			_ = os.Remove(temporaryPath)
			return ProjectEnvironment{}, errors.New("daemon: environment revision conflict")
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return ProjectEnvironment{}, fmt.Errorf("daemon: replace project environment: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return ProjectEnvironment{}, err
	}
	return loadProjectEnvironmentFile(ctx, directory, environment.ID)
}

func DeleteProjectEnvironment(ctx context.Context, folder, id, expectedHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := projectEnvironmentDirectory(folder, false)
	if err != nil {
		return err
	}
	if err := validateEnvironmentID(id); err != nil {
		return err
	}
	path := filepath.Join(directory, id)
	lock := environmentPathLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, readErr := readRegularEnvironmentFile(path, id)
	if readErr != nil {
		return fmt.Errorf("daemon: read environment for deletion: %w", readErr)
	}
	if expectedHash != "" && contentHash(current) != expectedHash {
		return errors.New("daemon: environment revision conflict")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("daemon: delete project environment: %w", err)
	}
	return syncDirectory(directory)
}

func loadProjectEnvironmentFile(ctx context.Context, directory, id string) (ProjectEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return ProjectEnvironment{}, err
	}
	if err := validateEnvironmentID(id); err != nil {
		return ProjectEnvironment{}, err
	}
	path := filepath.Join(directory, id)
	data, err := readRegularEnvironmentFile(path, id)
	if err != nil {
		return ProjectEnvironment{}, err
	}
	var environment ProjectEnvironment
	metadata, err := toml.Decode(string(data), &environment)
	if err != nil {
		return ProjectEnvironment{}, fmt.Errorf("daemon: decode project environment %q: %w", id, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) != 0 {
		return ProjectEnvironment{}, fmt.Errorf("daemon: project environment %q contains unknown field %q", id, undecoded[0].String())
	}
	environment.ID = id
	environment.ConfigPath = path
	environment.Hash = contentHash(data)
	if err := validateProjectEnvironment(environment); err != nil {
		return ProjectEnvironment{}, fmt.Errorf("daemon: project environment %q: %w", id, err)
	}
	assignEnvironmentActionIDs(&environment)
	return environment, nil
}

func readRegularEnvironmentFile(path, id string) ([]byte, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("daemon: inspect project environment %q: %w", id, err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("daemon: project environment %q must be a regular file", id)
	}
	if pathInfo.Size() > maxEnvironmentBytes {
		return nil, fmt.Errorf("daemon: project environment %q is too large", id)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("daemon: read project environment %q: %w", id, err)
	}
	defer file.Close()
	openInfo, err := file.Stat()
	if err != nil || !openInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openInfo) {
		return nil, fmt.Errorf("daemon: project environment %q changed while opening", id)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxEnvironmentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("daemon: read project environment %q: %w", id, err)
	}
	if len(data) > maxEnvironmentBytes {
		return nil, fmt.Errorf("daemon: project environment %q is too large", id)
	}
	return data, nil
}

func environmentPathLock(path string) *sync.Mutex {
	lock, _ := environmentPathLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func validateProjectEnvironment(environment ProjectEnvironment) error {
	if environment.Version != 1 {
		return errors.New("version must be 1")
	}
	if err := validateEnvironmentText(environment.Name, "name", 256, true); err != nil {
		return err
	}
	if err := validateEnvironmentScript(environment.Setup, "setup"); err != nil {
		return err
	}
	if environment.Cleanup != nil {
		if err := validateEnvironmentScript(*environment.Cleanup, "cleanup"); err != nil {
			return err
		}
	}
	if len(environment.Actions) > maxEnvironmentActions {
		return fmt.Errorf("cannot contain more than %d actions", maxEnvironmentActions)
	}
	for index, action := range environment.Actions {
		if err := validateEnvironmentText(action.Name, fmt.Sprintf("action %d name", index+1), 256, true); err != nil {
			return err
		}
		if err := validateEnvironmentText(action.Command, fmt.Sprintf("action %d command", index+1), maxEnvironmentBytes, true); err != nil {
			return err
		}
		switch action.Icon {
		case "", "tool", "run", "debug", "test":
		default:
			return fmt.Errorf("action %d has invalid icon %q", index+1, action.Icon)
		}
		if action.Platform != "" && !validEnvironmentPlatform(action.Platform) {
			return fmt.Errorf("action %d has invalid platform %q", index+1, action.Platform)
		}
	}
	return nil
}

func validateEnvironmentScript(script EnvironmentScript, label string) error {
	if err := validateEnvironmentText(script.Script, label+" script", maxEnvironmentBytes, false); err != nil {
		return err
	}
	for platform, value := range map[string]*EnvironmentPlatformScript{
		"darwin": script.Darwin, "linux": script.Linux, "win32": script.Win32,
	} {
		if value != nil {
			if err := validateEnvironmentText(value.Script, label+" "+platform+" script", maxEnvironmentBytes, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEnvironmentText(value, label string, max int, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("%s is invalid", label)
	}
	if len(value) > max {
		return fmt.Errorf("%s is too large", label)
	}
	return nil
}

func validateEnvironmentID(id string) error {
	if filepath.Base(id) != id || !environmentIDPattern.MatchString(id) {
		return fmt.Errorf("daemon: invalid environment id %q", id)
	}
	return nil
}

func projectEnvironmentDirectory(folder string, create bool) (string, error) {
	root, err := canonicalProjectFolder(folder)
	if err != nil {
		return "", err
	}
	directory := root
	for _, component := range []string{".codex", "environments"} {
		directory = filepath.Join(directory, component)
		info, statErr := os.Lstat(directory)
		if errors.Is(statErr, os.ErrNotExist) && create {
			if err := os.Mkdir(directory, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("daemon: create project environments directory: %w", err)
			}
			info, statErr = os.Lstat(directory)
		}
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return "", fmt.Errorf("daemon: inspect project environments directory: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", errors.New("daemon: project environments path must be a directory without symlinks")
		}
	}
	return directory, nil
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("daemon: open project environments directory: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("daemon: sync project environments directory: %w", err)
	}
	return nil
}

func assignEnvironmentActionIDs(environment *ProjectEnvironment) {
	for index := range environment.Actions {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", environment.ID, index, environment.Actions[index].Name)))
		environment.Actions[index].ID = fmt.Sprintf("env-%x", sum[:6])
	}
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func currentEnvironmentPlatform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

func validEnvironmentPlatform(platform string) bool {
	return platform == "darwin" || platform == "linux" || platform == "win32"
}

func environmentScriptForPlatform(script EnvironmentScript, platform string) string {
	var override *EnvironmentPlatformScript
	switch platform {
	case "darwin":
		override = script.Darwin
	case "linux":
		override = script.Linux
	case "win32":
		override = script.Win32
	}
	if override != nil && override.Script != "" {
		return override.Script
	}
	return script.Script
}

func sortProjectEnvironments(environments []ProjectEnvironment) {
	for index := 1; index < len(environments); index++ {
		for current := index; current > 0; current-- {
			left, right := environments[current-1], environments[current]
			leftDefault, rightDefault := left.ID == defaultEnvironmentID, right.ID == defaultEnvironmentID
			if leftDefault || (!rightDefault && left.ID <= right.ID) {
				break
			}
			environments[current-1], environments[current] = right, left
		}
	}
}
