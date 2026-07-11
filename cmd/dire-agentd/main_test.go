package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDataDirectoryPrefersCurrentAndFallsBackToLegacy(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	projects := filepath.Join(home, ".dire-agent", "projects")
	threads := filepath.Join(home, ".dire-agent", "threads")
	legacyProjects := filepath.Join(home, ".goagent", "projects")
	legacyThreads := filepath.Join(home, ".goagent", "threads")

	if got := defaultDataDirectory(home); got != projects {
		t.Fatalf("empty home data directory = %q, want %q", got, projects)
	}
	if err := os.MkdirAll(legacyThreads, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := defaultDataDirectory(home); got != legacyThreads {
		t.Fatalf("legacy threads fallback = %q, want %q", got, legacyThreads)
	}
	if err := os.MkdirAll(legacyProjects, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := defaultDataDirectory(home); got != legacyProjects {
		t.Fatalf("legacy projects fallback = %q, want %q", got, legacyProjects)
	}
	if err := os.MkdirAll(threads, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := defaultDataDirectory(home); got != threads {
		t.Fatalf("current threads fallback = %q, want %q", got, threads)
	}
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := defaultDataDirectory(home); got != projects {
		t.Fatalf("project preference = %q, want %q", got, projects)
	}
}

func TestLoadWebUIFromDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("production ui"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, source, err := loadWebUI(root)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(files, "index.html")
	if err != nil || string(contents) != "production ui" {
		t.Fatalf("index = %q, %v", contents, err)
	}
	canonical, _ := filepath.EvalSymlinks(root)
	if source != canonical {
		t.Fatalf("source = %q, want %q", source, canonical)
	}
}

func TestLoadWebUIRejectsInvalidDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, _, err := loadWebUI(root); err == nil {
		t.Fatal("accepted Web UI directory without index.html")
	}
	file := filepath.Join(root, "bundle")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadWebUI(file); err == nil {
		t.Fatal("accepted a Web UI file as a directory")
	}
}

func TestAddressPort(t *testing.T) {
	t.Parallel()
	tests := map[string]int{
		"127.0.0.1:7331": 7331,
		"localhost:5172": 5172,
		"[::1]:8080":     8080,
		"127.0.0.1:0":    0,
		"not-an-address": 0,
	}
	for address, want := range tests {
		if got := addressPort(address); got != want {
			t.Errorf("addressPort(%q) = %d, want %d", address, got, want)
		}
	}
}

func TestValidateListenerSecurity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		address                 string
		allowRemote             bool
		projectProxy            bool
		allowRemoteProjectProxy bool
		wantError               bool
	}{
		{name: "loopback proxy", address: "127.0.0.1:7331", projectProxy: true},
		{name: "remote refused", address: "0.0.0.0:7331", projectProxy: false, wantError: true},
		{name: "remote API explicit", address: "0.0.0.0:7331", allowRemote: true},
		{name: "remote proxy needs second opt in", address: "0.0.0.0:7331", allowRemote: true, projectProxy: true, wantError: true},
		{name: "remote proxy explicit", address: "0.0.0.0:7331", allowRemote: true, projectProxy: true, allowRemoteProjectProxy: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateListenerSecurity(test.address, test.allowRemote, test.projectProxy, test.allowRemoteProjectProxy)
			if (err != nil) != test.wantError {
				t.Fatalf("validateListenerSecurity() error = %v, want error %v", err, test.wantError)
			}
		})
	}
}
