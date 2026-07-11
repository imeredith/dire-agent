package daemon_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/daemon"
)

func TestWebUIHostingAndSPAFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":             "<!doctype html><title>dire-agent production</title><div id=\"root\"></div>",
		"assets/app-deadbeef.js": "globalThis.direAgentProduction = true",
		"favicon.svg":            "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler := (&daemon.Server{WebUI: os.DirFS(root)}).Handler()

	tests := []struct {
		name        string
		method      string
		path        string
		accept      string
		status      int
		body        string
		cache       string
		contentType string
	}{
		{name: "root index", method: http.MethodGet, path: "/", status: http.StatusOK, body: "dire-agent production", cache: "no-cache", contentType: "text/html"},
		{name: "spa route", method: http.MethodGet, path: "/docs/folder-projects", accept: "text/html", status: http.StatusOK, body: "dire-agent production", cache: "no-cache", contentType: "text/html"},
		{name: "hashed asset", method: http.MethodGet, path: "/assets/app-deadbeef.js", status: http.StatusOK, body: "direAgentProduction", cache: "public, max-age=31536000, immutable", contentType: "text/javascript"},
		{name: "ordinary asset", method: http.MethodGet, path: "/favicon.svg", status: http.StatusOK, body: "<svg", cache: "public, max-age=3600", contentType: "image/svg+xml"},
		{name: "missing hashed asset", method: http.MethodGet, path: "/assets/missing.js", status: http.StatusNotFound},
		{name: "asset directory", method: http.MethodGet, path: "/assets", status: http.StatusNotFound},
		{name: "missing file", method: http.MethodGet, path: "/missing.css", status: http.StatusNotFound},
		{name: "non html fetch", method: http.MethodGet, path: "/unknown", accept: "application/json", status: http.StatusNotFound},
		{name: "mutation", method: http.MethodPost, path: "/", status: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if test.accept != "" {
				request.Header.Set("Accept", test.accept)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, test.status, recorder.Body.String())
			}
			if test.body != "" && !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("body = %q, want containing %q", recorder.Body.String(), test.body)
			}
			if test.cache != "" && recorder.Header().Get("Cache-Control") != test.cache {
				t.Fatalf("cache = %q, want %q", recorder.Header().Get("Cache-Control"), test.cache)
			}
			if test.contentType != "" && !strings.Contains(recorder.Header().Get("Content-Type"), test.contentType) {
				t.Fatalf("content type = %q, want containing %q", recorder.Header().Get("Content-Type"), test.contentType)
			}
			if test.status == http.StatusOK && recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("successful Web UI response did not disable MIME sniffing")
			}
		})
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/assets/app-deadbeef.js", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD response = %d body=%q", head.Code, head.Body.String())
	}
}

func TestWebUIHostingIsOptIn(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	(&daemon.Server{}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("API-only root status = %d, want 404", recorder.Code)
	}
}
