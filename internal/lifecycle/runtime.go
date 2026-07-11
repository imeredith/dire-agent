package lifecycle

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// RuntimeSchema is the current on-disk daemon runtime file format.
	RuntimeSchema = 1
	// ServiceName identifies Dire Agent health responses.
	ServiceName = "dire-agent"
	// DefaultAddress is the loopback address used by a supervised daemon.
	DefaultAddress = "127.0.0.1:7331"
)

// RuntimeState is the private, daemon-owned state used to safely manage a
// specific daemon process. ControlToken must never be printed or returned by a
// public status command.
type RuntimeState struct {
	Schema       int       `json:"schema"`
	PID          int       `json:"pid"`
	InstanceID   string    `json:"instance_id"`
	ControlToken string    `json:"control_token"`
	Version      string    `json:"version"`
	Executable   string    `json:"executable"`
	HTTPURL      string    `json:"http_url"`
	StartedAt    time.Time `json:"started_at"`
}

// Health is the identity-bearing response returned by the daemon health
// endpoint.
type Health struct {
	Service    string `json:"service"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	PID        int    `json:"pid"`
	InstanceID string `json:"instance_id"`
}

// Paths contains all private state paths used by the lifecycle supervisor.
type Paths struct {
	BaseDir           string
	RunDir            string
	RuntimeFile       string
	LockFile          string
	OperationLockFile string
	LogDir            string
	LogFile           string
}

// DefaultPaths returns lifecycle paths rooted below home.
func DefaultPaths(home string) Paths {
	base := filepath.Join(home, ".dire-agent")
	run := filepath.Join(base, "run")
	logs := filepath.Join(base, "logs")
	return Paths{
		BaseDir:           base,
		RunDir:            run,
		RuntimeFile:       filepath.Join(run, "daemon.json"),
		LockFile:          filepath.Join(run, "lifecycle.lock"),
		OperationLockFile: filepath.Join(run, "operation.lock"),
		LogDir:            logs,
		LogFile:           filepath.Join(logs, "daemon.log"),
	}
}

// NewRandomToken creates a control token with 256 bits of entropy.
func NewRandomToken() (string, error) {
	return randomString(32)
}

// NewInstanceID creates an opaque identifier for one daemon process.
func NewInstanceID() (string, error) {
	return randomString(18)
}

func randomString(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// ValidateRuntime verifies that state is complete and only references an HTTP
// service on the local machine.
func ValidateRuntime(state RuntimeState) error {
	if state.Schema != RuntimeSchema {
		return fmt.Errorf("unsupported runtime schema %d", state.Schema)
	}
	if state.PID <= 1 {
		return fmt.Errorf("invalid daemon pid %d", state.PID)
	}
	if len(state.InstanceID) < 16 {
		return errors.New("invalid daemon instance id")
	}
	if len(state.ControlToken) < 32 {
		return errors.New("invalid daemon control token")
	}
	if strings.TrimSpace(state.Version) == "" {
		return errors.New("daemon version is empty")
	}
	if !filepath.IsAbs(state.Executable) {
		return errors.New("daemon executable path is not absolute")
	}
	if state.StartedAt.IsZero() {
		return errors.New("daemon start time is empty")
	}
	if err := validateLoopbackURL(state.HTTPURL); err != nil {
		return fmt.Errorf("invalid daemon HTTP URL: %w", err)
	}
	return nil
}

func validateLoopbackURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" {
		return errors.New("scheme must be http")
	}
	if parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("URL must be a plain loopback HTTP origin")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return errors.New("URL must not include a path")
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("host is not loopback")
	}
	return nil
}

// WriteRuntime atomically writes state with owner-only permissions.
func WriteRuntime(path string, state RuntimeState) error {
	if err := ValidateRuntime(state); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := ensurePrivateDir(directory); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(directory, ".daemon-*.json")
	if err != nil {
		return fmt.Errorf("create temporary runtime file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary runtime file: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode runtime state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync runtime state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close runtime state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install runtime state: %w", err)
	}
	committed = true
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure runtime state: %w", err)
	}
	if directoryHandle, err := os.Open(directory); err == nil {
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

// ReadRuntime reads and validates a daemon runtime file.
func ReadRuntime(path string) (RuntimeState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return RuntimeState{}, err
	}
	if !info.Mode().IsRegular() {
		return RuntimeState{}, errors.New("runtime state is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return RuntimeState{}, errors.New("runtime state permissions are not private")
	}
	file, err := os.Open(path)
	if err != nil {
		return RuntimeState{}, err
	}
	defer file.Close()

	var state RuntimeState
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return RuntimeState{}, fmt.Errorf("decode runtime state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return RuntimeState{}, errors.New("decode runtime state: trailing content")
		}
		return RuntimeState{}, fmt.Errorf("decode runtime state trailing content: %w", err)
	}
	if err := ValidateRuntime(state); err != nil {
		return RuntimeState{}, err
	}
	return state, nil
}

// RemoveRuntimeIfInstance removes path only if it still belongs to instanceID.
// It therefore cannot erase runtime state written by a replacement daemon.
func RemoveRuntimeIfInstance(path, instanceID string) (bool, error) {
	if instanceID == "" {
		return false, errors.New("instance id is empty")
	}
	// Move the file out of the live name first. Reading and then removing path
	// would have a race in which a replacement daemon could install its state
	// between those operations. The private quarantine directory lets us inspect
	// exactly the file moved by this call.
	quarantineDir, err := os.MkdirTemp(filepath.Dir(path), ".runtime-remove-")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("create runtime quarantine: %w", err)
	}
	if err := os.Chmod(quarantineDir, 0o700); err != nil {
		_ = os.Remove(quarantineDir)
		return false, fmt.Errorf("secure runtime quarantine: %w", err)
	}
	quarantinePath := filepath.Join(quarantineDir, filepath.Base(path))
	if err := os.Rename(path, quarantinePath); err != nil {
		_ = os.Remove(quarantineDir)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("quarantine runtime state: %w", err)
	}

	state, readErr := ReadRuntime(quarantinePath)
	if readErr != nil || state.InstanceID != instanceID {
		restoreErr := restoreQuarantinedRuntime(quarantinePath, path)
		_ = os.Remove(quarantineDir)
		if readErr != nil {
			if restoreErr != nil {
				return false, fmt.Errorf("read quarantined runtime state: %v (also failed to restore it: %w)", readErr, restoreErr)
			}
			return false, readErr
		}
		if restoreErr != nil {
			return false, fmt.Errorf("restore runtime state for a different instance: %w", restoreErr)
		}
		return false, nil
	}
	if err := os.Remove(quarantinePath); err != nil {
		return false, fmt.Errorf("remove quarantined runtime state: %w", err)
	}
	_ = os.Remove(quarantineDir)
	return true, nil
}

func restoreQuarantinedRuntime(quarantinePath, path string) error {
	// Link is an atomic create-if-absent operation. If a replacement daemon has
	// already installed state, preserve that live file and discard only the
	// quarantined older copy.
	err := os.Link(quarantinePath, path)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := os.Remove(quarantinePath); err != nil {
		return err
	}
	return nil
}

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create private directory %s: %w", path, err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect private directory %s: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private path is not a real directory: %s", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private directory permissions are too broad: %s", path)
	}
	return nil
}

// ensureApplicationPrivateDir may repair permissions only on directories the
// supervisor derives beneath the user's home. Arbitrary runtime-file parents
// use ensurePrivateDir and are never chmodded as a side effect.
func ensureApplicationPrivateDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create private directory %s: %w", path, err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect private directory %s: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private path is not a real directory: %s", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure private directory %s: %w", path, err)
	}
	return nil
}
