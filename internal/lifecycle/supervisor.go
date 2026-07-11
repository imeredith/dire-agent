package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultStartTimeout = 12 * time.Second
	defaultStopTimeout  = 10 * time.Second
	defaultPollInterval = 75 * time.Millisecond
	maxHealthBody       = 64 << 10
)

// Status describes the daemon without exposing its private control token.
type Status struct {
	Running    bool
	Managed    bool
	PID        int
	Version    string
	HTTPURL    string
	LogFile    string
	InstanceID string
	Healthy    bool
	Stale      bool
	Detail     string
}

// Supervisor starts and stops one user-scoped Dire Agent daemon.
type Supervisor struct {
	Home         string
	Executable   string
	Address      string
	Paths        Paths
	Client       *http.Client
	StartTimeout time.Duration
	StopTimeout  time.Duration
	PollInterval time.Duration

	processAlive func(int) (bool, error)
}

// OperationLock serializes multi-step user-facing lifecycle operations such
// as an upgrade that stops, replaces, and restarts the daemon.
type OperationLock struct {
	lock *fileLock
}

// AcquireOperationLock acquires the user-scoped command lock. Call Close when
// the complete lifecycle transaction has finished.
func (supervisor *Supervisor) AcquireOperationLock(ctx context.Context) (*OperationLock, error) {
	if err := supervisor.validate(); err != nil {
		return nil, err
	}
	if err := supervisor.ensureDirectories(); err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(ctx, supervisor.Paths.OperationLockFile)
	if err != nil {
		return nil, err
	}
	return &OperationLock{lock: lock}, nil
}

func (lock *OperationLock) Close() error {
	if lock == nil {
		return nil
	}
	return lock.lock.close()
}

// NewSupervisor constructs a supervisor for executable. Empty home and
// executable values are resolved from the current user and process.
func NewSupervisor(home, executable string) (*Supervisor, error) {
	var err error
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable: %w", err)
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}

	client := &http.Client{
		Timeout: time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("daemon endpoint redirected")
		},
	}
	return &Supervisor{
		Home:         home,
		Executable:   executable,
		Address:      DefaultAddress,
		Paths:        DefaultPaths(home),
		Client:       client,
		StartTimeout: defaultStartTimeout,
		StopTimeout:  defaultStopTimeout,
		PollInterval: defaultPollInterval,
		processAlive: osProcessAlive,
	}, nil
}

// Start starts a detached daemon if no healthy daemon is already present. The
// returned bool reports whether this call launched a new process.
func (supervisor *Supervisor) Start(ctx context.Context) (bool, error) {
	if err := supervisor.validate(); err != nil {
		return false, err
	}
	if err := supervisor.ensureDirectories(); err != nil {
		return false, err
	}
	lock, err := acquireFileLock(ctx, supervisor.Paths.LockFile)
	if err != nil {
		return false, err
	}
	defer lock.close()

	state, err := ReadRuntime(supervisor.Paths.RuntimeFile)
	switch {
	case err == nil:
		alive, aliveErr := supervisor.processAlive(state.PID)
		if aliveErr != nil {
			return false, aliveErr
		}
		if !alive {
			if _, removeErr := RemoveRuntimeIfInstance(supervisor.Paths.RuntimeFile, state.InstanceID); removeErr != nil {
				return false, fmt.Errorf("remove stale runtime state: %w", removeErr)
			}
			break
		}
		health, healthErr := supervisor.fetchHealth(ctx, state.HTTPURL)
		if healthErr != nil {
			return false, fmt.Errorf("daemon pid %d is alive but its identity could not be verified: %w", state.PID, healthErr)
		}
		if err := matchIdentity(state, health); err != nil {
			return false, fmt.Errorf("refusing to replace daemon pid %d: %w", state.PID, err)
		}
		return false, nil
	case errors.Is(err, os.ErrNotExist):
		// No managed daemon state exists.
	default:
		return false, fmt.Errorf("read daemon runtime state: %w", err)
	}

	// Treat a healthy, directly started daemon as already running. It cannot be
	// stopped by this supervisor because there is deliberately no control token.
	if health, healthErr := supervisor.fetchHealth(ctx, supervisor.defaultHTTPURL()); healthErr == nil && validHealth(health) == nil {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	instanceID, err := NewInstanceID()
	if err != nil {
		return false, err
	}
	controlToken, err := NewRandomToken()
	if err != nil {
		return false, err
	}

	logFile, err := os.OpenFile(supervisor.Paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return false, fmt.Errorf("open daemon log: %w", err)
	}
	if err := logFile.Chmod(0o600); err != nil {
		_ = logFile.Close()
		return false, fmt.Errorf("secure daemon log: %w", err)
	}
	nullFile, err := os.Open(os.DevNull)
	if err != nil {
		_ = logFile.Close()
		return false, fmt.Errorf("open null input: %w", err)
	}

	command := exec.Command(
		supervisor.Executable,
		"daemon",
		"-runtime-file", supervisor.Paths.RuntimeFile,
		"-instance-id", instanceID,
		"-addr", supervisor.Address,
	)
	command.Dir = supervisor.Home
	command.Env = append(os.Environ(), "DIRE_AGENT_CONTROL_TOKEN="+controlToken)
	command.Stdin = nullFile
	command.Stdout = logFile
	command.Stderr = logFile
	if err := configureDetached(command); err != nil {
		_ = nullFile.Close()
		_ = logFile.Close()
		return false, err
	}
	if err := command.Start(); err != nil {
		_ = nullFile.Close()
		_ = logFile.Close()
		return false, fmt.Errorf("start daemon: %w", err)
	}
	pid := command.Process.Pid
	// Reap the child while this launcher remains alive (notably while its TUI is
	// open). If the launcher exits first, the detached session is reparented and
	// reaped by the operating system.
	go func() { _ = command.Wait() }()
	_ = nullFile.Close()
	_ = logFile.Close()

	startCtx, cancel := context.WithTimeout(ctx, supervisor.StartTimeout)
	defer cancel()
	for {
		state, readErr := ReadRuntime(supervisor.Paths.RuntimeFile)
		if readErr == nil {
			if state.InstanceID != instanceID || state.PID != pid || state.ControlToken != controlToken {
				return true, errors.New("another daemon replaced the runtime state during startup")
			}
			health, healthErr := supervisor.fetchHealth(startCtx, state.HTTPURL)
			if healthErr == nil && matchIdentity(state, health) == nil {
				return true, nil
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return true, fmt.Errorf("read daemon runtime state during startup: %w", readErr)
		}

		alive, aliveErr := supervisor.processAlive(pid)
		if aliveErr != nil {
			return true, aliveErr
		}
		if !alive {
			return true, fmt.Errorf("daemon exited during startup; see %s", supervisor.Paths.LogFile)
		}
		select {
		case <-startCtx.Done():
			return true, fmt.Errorf("daemon did not become healthy: %w; see %s", startCtx.Err(), supervisor.Paths.LogFile)
		case <-time.After(supervisor.PollInterval):
		}
	}
}

// Stop requests authenticated shutdown of the managed daemon. It never sends
// a signal to a PID from disk. The returned bool reports whether a verified
// running daemon was asked to stop.
func (supervisor *Supervisor) Stop(ctx context.Context) (bool, error) {
	if err := supervisor.validate(); err != nil {
		return false, err
	}
	if err := supervisor.ensureDirectories(); err != nil {
		return false, err
	}
	lock, err := acquireFileLock(ctx, supervisor.Paths.LockFile)
	if err != nil {
		return false, err
	}
	defer lock.close()

	state, err := ReadRuntime(supervisor.Paths.RuntimeFile)
	if errors.Is(err, os.ErrNotExist) {
		health, healthErr := supervisor.fetchHealth(ctx, supervisor.defaultHTTPURL())
		if healthErr == nil && validHealth(health) == nil {
			return false, fmt.Errorf("Dire Agent daemon pid %d is running without supervisor state; refusing unauthenticated shutdown", health.PID)
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read daemon runtime state: %w", err)
	}
	alive, err := supervisor.processAlive(state.PID)
	if err != nil {
		return false, err
	}
	if !alive {
		_, removeErr := RemoveRuntimeIfInstance(supervisor.Paths.RuntimeFile, state.InstanceID)
		if removeErr != nil {
			return false, fmt.Errorf("remove stale runtime state: %w", removeErr)
		}
		return false, nil
	}

	stopCtx, cancel := context.WithTimeout(ctx, supervisor.StopTimeout)
	defer cancel()
	health, err := supervisor.fetchHealth(stopCtx, state.HTTPURL)
	if err != nil {
		return false, fmt.Errorf("refusing to stop pid %d because daemon identity could not be verified: %w", state.PID, err)
	}
	if err := matchIdentity(state, health); err != nil {
		return false, fmt.Errorf("refusing to stop pid %d: %w", state.PID, err)
	}

	shutdownURL, err := endpointURL(state.HTTPURL, "/control/shutdown")
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(stopCtx, http.MethodPost, shutdownURL, nil)
	if err != nil {
		return false, fmt.Errorf("create shutdown request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+state.ControlToken)
	response, err := supervisor.Client.Do(request)
	if err != nil {
		return false, fmt.Errorf("request daemon shutdown: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	closeErr := response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("daemon rejected shutdown with HTTP %s", response.Status)
	}
	if closeErr != nil {
		return true, fmt.Errorf("close shutdown response: %w", closeErr)
	}

	for {
		alive, aliveErr := supervisor.processAlive(state.PID)
		if aliveErr != nil {
			return true, aliveErr
		}
		if !alive {
			_, removeErr := RemoveRuntimeIfInstance(supervisor.Paths.RuntimeFile, state.InstanceID)
			if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return true, fmt.Errorf("remove stopped daemon runtime state: %w", removeErr)
			}
			return true, nil
		}
		select {
		case <-stopCtx.Done():
			return true, fmt.Errorf("daemon did not stop: %w", stopCtx.Err())
		case <-time.After(supervisor.PollInterval):
		}
	}
}

// Status inspects runtime and health identity without changing daemon state.
func (supervisor *Supervisor) Status(ctx context.Context) (Status, error) {
	if err := supervisor.validate(); err != nil {
		return Status{}, err
	}
	result := Status{LogFile: supervisor.Paths.LogFile}
	state, err := ReadRuntime(supervisor.Paths.RuntimeFile)
	if errors.Is(err, os.ErrNotExist) {
		health, healthErr := supervisor.fetchHealth(ctx, supervisor.defaultHTTPURL())
		if healthErr == nil && validHealth(health) == nil {
			result.Running = true
			result.Healthy = true
			result.PID = health.PID
			result.Version = health.Version
			result.HTTPURL = supervisor.defaultHTTPURL()
			result.InstanceID = health.InstanceID
			result.Detail = "running without supervisor state"
		}
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read daemon runtime state: %w", err)
	}
	result.Managed = true
	result.PID = state.PID
	result.Version = state.Version
	result.HTTPURL = state.HTTPURL
	result.InstanceID = state.InstanceID

	alive, err := supervisor.processAlive(state.PID)
	if err != nil {
		return result, err
	}
	if !alive {
		result.Stale = true
		result.Detail = "stale runtime state; process is not alive"
		return result, nil
	}
	health, err := supervisor.fetchHealth(ctx, state.HTTPURL)
	if err != nil {
		result.Detail = "process is alive but health check failed: " + err.Error()
		return result, nil
	}
	if err := matchIdentity(state, health); err != nil {
		result.Detail = "process is alive but identity does not match: " + err.Error()
		return result, nil
	}
	result.Running = true
	result.Healthy = true
	result.Detail = "running"
	return result, nil
}

// IsRunning reports whether a daemon with a verified health identity is up.
func (supervisor *Supervisor) IsRunning(ctx context.Context) (bool, error) {
	status, err := supervisor.Status(ctx)
	return status.Running, err
}

func (supervisor *Supervisor) validate() error {
	if supervisor == nil {
		return errors.New("lifecycle supervisor is nil")
	}
	if supervisor.Home == "" || !filepath.IsAbs(supervisor.Home) {
		return errors.New("supervisor home must be an absolute path")
	}
	if supervisor.Executable == "" || !filepath.IsAbs(supervisor.Executable) {
		return errors.New("supervisor executable must be an absolute path")
	}
	if _, err := url.ParseRequestURI(supervisor.defaultHTTPURL()); err != nil {
		return fmt.Errorf("invalid daemon address: %w", err)
	}
	if err := validateLoopbackURL(supervisor.defaultHTTPURL()); err != nil {
		return fmt.Errorf("invalid daemon address: %w", err)
	}
	if supervisor.Client == nil {
		return errors.New("supervisor HTTP client is nil")
	}
	if supervisor.StartTimeout <= 0 || supervisor.StopTimeout <= 0 || supervisor.PollInterval <= 0 {
		return errors.New("supervisor timeouts must be positive")
	}
	if supervisor.processAlive == nil {
		return errors.New("supervisor process checker is nil")
	}
	return nil
}

func (supervisor *Supervisor) ensureDirectories() error {
	for _, directory := range []string{supervisor.Paths.BaseDir, supervisor.Paths.RunDir, supervisor.Paths.LogDir} {
		if err := ensureApplicationPrivateDir(directory); err != nil {
			return err
		}
	}
	return nil
}

func (supervisor *Supervisor) defaultHTTPURL() string {
	return "http://" + supervisor.Address
}

func (supervisor *Supervisor) fetchHealth(ctx context.Context, baseURL string) (Health, error) {
	healthURL, err := endpointURL(baseURL, "/healthz")
	if err != nil {
		return Health{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return Health{}, fmt.Errorf("create health request: %w", err)
	}
	response, err := supervisor.Client.Do(request)
	if err != nil {
		return Health{}, fmt.Errorf("request daemon health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return Health{}, fmt.Errorf("health endpoint returned HTTP %s", response.Status)
	}
	var health Health
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxHealthBody))
	if err := decoder.Decode(&health); err != nil {
		return Health{}, fmt.Errorf("decode daemon health: %w", err)
	}
	if err := validHealth(health); err != nil {
		return Health{}, err
	}
	return health, nil
}

func validHealth(health Health) error {
	if health.Service != ServiceName {
		return fmt.Errorf("unexpected service %q", health.Service)
	}
	if health.Status != "ok" {
		return fmt.Errorf("daemon status is %q", health.Status)
	}
	if health.PID <= 1 {
		return fmt.Errorf("invalid health pid %d", health.PID)
	}
	if len(health.InstanceID) < 16 {
		return errors.New("invalid health instance id")
	}
	if strings.TrimSpace(health.Version) == "" {
		return errors.New("health version is empty")
	}
	return nil
}

func matchIdentity(state RuntimeState, health Health) error {
	if err := validHealth(health); err != nil {
		return err
	}
	if health.PID != state.PID {
		return fmt.Errorf("health pid %d does not match runtime pid %d", health.PID, state.PID)
	}
	if health.InstanceID != state.InstanceID {
		return errors.New("health instance id does not match runtime state")
	}
	if health.Version != state.Version {
		return fmt.Errorf("health version %q does not match runtime version %q", health.Version, state.Version)
	}
	return nil
}

func endpointURL(baseURL, endpoint string) (string, error) {
	if err := validateLoopbackURL(baseURL); err != nil {
		return "", fmt.Errorf("unsafe daemon URL: %w", err)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = endpoint
	return parsed.String(), nil
}
