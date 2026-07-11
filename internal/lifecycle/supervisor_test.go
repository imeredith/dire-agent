package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartIsIdempotentForVerifiedManagedDaemon(t *testing.T) {
	const instanceID = "instance-1234567890"
	health := Health{Service: ServiceName, Status: "ok", Version: "v1.2.3", PID: 4242, InstanceID: instanceID}
	supervisor := testSupervisor(t, mockClient(t, health, nil))
	state := testRuntimeState(supervisor.Paths.RuntimeFile, instanceID)
	if err := WriteRuntime(supervisor.Paths.RuntimeFile, state); err != nil {
		t.Fatal(err)
	}
	supervisor.processAlive = func(pid int) (bool, error) { return pid == state.PID, nil }

	started, err := supervisor.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started {
		t.Fatal("Start reported a newly launched daemon")
	}
}

func TestStatusIdentifiesManagedDaemon(t *testing.T) {
	const instanceID = "instance-1234567890"
	health := Health{Service: ServiceName, Status: "ok", Version: "v1.2.3", PID: 4242, InstanceID: instanceID}
	supervisor := testSupervisor(t, mockClient(t, health, nil))
	state := testRuntimeState(supervisor.Paths.RuntimeFile, instanceID)
	if err := WriteRuntime(supervisor.Paths.RuntimeFile, state); err != nil {
		t.Fatal(err)
	}
	supervisor.processAlive = func(int) (bool, error) { return true, nil }

	status, err := supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Running || !status.Managed || !status.Healthy {
		t.Fatalf("Status = %#v, want running managed healthy", status)
	}
	if status.PID != state.PID || status.Version != state.Version || status.HTTPURL != state.HTTPURL {
		t.Fatalf("Status identity = %#v, want state %#v", status, state)
	}
}

func TestStopUsesAuthenticatedEndpointAndNeverSignalsPID(t *testing.T) {
	const instanceID = "instance-1234567890"
	const token = "control-token-control-token-control-token"
	var alive atomic.Bool
	alive.Store(true)
	var runtimePath string
	var authorized atomic.Bool
	health := Health{Service: ServiceName, Status: "ok", Version: "v1.2.3", PID: 4242, InstanceID: instanceID}
	client := mockClient(t, health, func(request *http.Request) int {
		if request.Header.Get("Authorization") == "Bearer "+token {
			authorized.Store(true)
		}
		alive.Store(false)
		_, _ = RemoveRuntimeIfInstance(runtimePath, instanceID)
		return http.StatusAccepted
	})

	supervisor := testSupervisor(t, client)
	runtimePath = supervisor.Paths.RuntimeFile
	state := testRuntimeState(runtimePath, instanceID)
	state.ControlToken = token
	if err := WriteRuntime(runtimePath, state); err != nil {
		t.Fatal(err)
	}
	supervisor.processAlive = func(int) (bool, error) { return alive.Load(), nil }

	stopped, err := supervisor.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !stopped {
		t.Fatal("Stop did not report a running daemon")
	}
	if !authorized.Load() {
		t.Fatal("Stop did not send the expected bearer token")
	}
}

func TestStopRefusesMismatchedIdentity(t *testing.T) {
	const instanceID = "instance-1234567890"
	var shutdownRequests atomic.Int32
	health := Health{Service: ServiceName, Status: "ok", Version: "v1.2.3", PID: 9999, InstanceID: instanceID}
	client := mockClient(t, health, func(request *http.Request) int {
		shutdownRequests.Add(1)
		return http.StatusAccepted
	})

	supervisor := testSupervisor(t, client)
	state := testRuntimeState(supervisor.Paths.RuntimeFile, instanceID)
	if err := WriteRuntime(supervisor.Paths.RuntimeFile, state); err != nil {
		t.Fatal(err)
	}
	supervisor.processAlive = func(int) (bool, error) { return true, nil }

	stopped, err := supervisor.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Stop error = %v, want identity mismatch", err)
	}
	if stopped {
		t.Fatal("Stop reported mismatched daemon as stopped")
	}
	if shutdownRequests.Load() != 0 {
		t.Fatal("Stop called shutdown endpoint for mismatched daemon")
	}
}

func TestStopRemovesStaleStateWithoutShutdown(t *testing.T) {
	supervisor := testSupervisor(t, mockClient(t, Health{}, nil))
	state := testRuntimeState(supervisor.Paths.RuntimeFile, "instance-1234567890")
	if err := WriteRuntime(supervisor.Paths.RuntimeFile, state); err != nil {
		t.Fatal(err)
	}
	supervisor.processAlive = func(int) (bool, error) { return false, nil }

	stopped, err := supervisor.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped {
		t.Fatal("stale process reported as running")
	}
	if _, err := os.Stat(supervisor.Paths.RuntimeFile); !os.IsNotExist(err) {
		t.Fatalf("stale runtime file still exists: %v", err)
	}
}

func TestStopRefusesHealthyUnmanagedDaemon(t *testing.T) {
	health := Health{
		Service:    ServiceName,
		Status:     "ok",
		Version:    "v1.2.3",
		PID:        4242,
		InstanceID: "instance-1234567890",
	}
	supervisor := testSupervisor(t, mockClient(t, health, nil))

	stopped, err := supervisor.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "without supervisor state") {
		t.Fatalf("Stop error = %v, want unmanaged-daemon refusal", err)
	}
	if stopped {
		t.Fatal("unmanaged daemon reported as stopped")
	}
}

func TestFileLockHonorsContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lifecycle.lock")
	first, err := acquireFileLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := acquireFileLock(ctx, path); err == nil {
		t.Fatal("second lock unexpectedly succeeded")
	}
}

func testSupervisor(t *testing.T, client *http.Client) *Supervisor {
	t.Helper()
	home := t.TempDir()
	supervisor, err := NewSupervisor(home, filepath.Join(home, "bin", "dire-agent"))
	if err != nil {
		t.Fatal(err)
	}
	supervisor.Client = client
	supervisor.StartTimeout = time.Second
	supervisor.StopTimeout = time.Second
	supervisor.PollInterval = time.Millisecond
	return supervisor
}

func mockClient(t *testing.T, health Health, shutdown func(*http.Request) int) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		status := http.StatusOK
		var body bytes.Buffer
		switch request.URL.Path {
		case "/healthz":
			if err := json.NewEncoder(&body).Encode(health); err != nil {
				t.Errorf("encode health: %v", err)
			}
		case "/control/shutdown":
			if shutdown == nil {
				status = http.StatusNotFound
				break
			}
			status = shutdown(request)
		default:
			status = http.StatusNotFound
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
			Request:    request,
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
