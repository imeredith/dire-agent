package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareAndApply(t *testing.T) {
	t.Parallel()
	oldBinary := []byte("old binary")
	newBinary := []byte("new binary")
	hash := sha256.Sum256(newBinary)
	target := filepath.Join(t.TempDir(), "dire-agent")
	if err := os.WriteFile(target, oldBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	u := New(target)
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.LatestBaseURL = "https://releases.example/latest"
	u.ReleaseBaseURL = "https://releases.example/download"
	u.Client = releaseClient(newBinary, fmt.Sprintf("%x  dire-agent-linux-amd64\n", hash))
	prepared, err := u.Prepare(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Cleanup()
	if got, _ := os.ReadFile(target); string(got) != string(oldBinary) {
		t.Fatalf("Prepare changed target to %q", got)
	}
	if prepared.Version != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", prepared.Version)
	}
	if err := prepared.Apply(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != string(newBinary) {
		t.Fatalf("Apply target = %q, want %q", got, newBinary)
	}
}

func TestPrepareRejectsChecksumMismatchWithoutMutation(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "dire-agent")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := New(target)
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.LatestBaseURL = "https://releases.example/latest"
	u.ReleaseBaseURL = "https://releases.example/download"
	u.Client = releaseClient([]byte("new binary"), fmt.Sprintf("%064x  dire-agent-linux-amd64\n", 0))
	if _, err := u.Prepare(context.Background(), ""); err == nil {
		t.Fatal("Prepare accepted a checksum mismatch")
	}
	if got, _ := os.ReadFile(target); string(got) != "old binary" {
		t.Fatalf("failed Prepare changed target to %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func releaseClient(binary []byte, checksums string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body []byte
		switch request.URL.Path {
		case "/latest/version.txt":
			body = []byte("v1.2.3\n")
		case "/download/v1.2.3/checksums.txt":
			body = []byte(checksums)
		case "/download/v1.2.3/dire-agent-linux-amd64":
			body = binary
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound, Status: "404 Not Found",
				Header: make(http.Header), Body: io.NopCloser(strings.NewReader("not found")), Request: request,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header),
			Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: request,
		}, nil
	})}
}
