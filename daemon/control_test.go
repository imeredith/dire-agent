package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/internal/lifecycle"
)

func TestHealthIdentifiesDaemon(t *testing.T) {
	t.Parallel()
	want := lifecycle.Health{
		Service: lifecycle.ServiceName, Status: "ok", Version: "v1.2.3", PID: 4321, InstanceID: "instance-a",
	}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	(&Server{Health: want}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
	var got lifecycle.Health
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("health = %#v, want %#v", got, want)
	}
}

func TestHealthRejectsMutation(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestShutdownRequiresToken(t *testing.T) {
	t.Parallel()
	called := make(chan struct{}, 1)
	handler := (&Server{ControlToken: "secret", Shutdown: func() { called <- struct{}{} }}).Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/control/shutdown", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	wrongMethod := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/control/shutdown", nil)
	request.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(wrongMethod, request)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong-method status = %d, want %d", wrongMethod.Code, http.StatusMethodNotAllowed)
	}

	authorized := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/control/shutdown", nil)
	request.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusAccepted {
		t.Fatalf("authorized status = %d, want %d", authorized.Code, http.StatusAccepted)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not invoked")
	}
}
