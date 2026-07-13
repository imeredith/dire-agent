package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

type recordingProvider struct {
	base    fakeProvider
	mu      sync.Mutex
	options []agent.SessionOptions
}

func (p *recordingProvider) record(options agent.SessionOptions) {
	p.mu.Lock()
	p.options = append(p.options, options)
	p.mu.Unlock()
}

func (p *recordingProvider) OpenSession(ctx context.Context, options agent.SessionOptions) (agent.Session, error) {
	p.record(options)
	return p.base.OpenSession(ctx, options)
}

func (p *recordingProvider) OpenSessionFromState(ctx context.Context, options agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	p.record(options)
	return p.base.OpenSessionFromState(ctx, options, state)
}

func (p *recordingProvider) Close() error { return p.base.Close() }

func (p *recordingProvider) lastOptions(t *testing.T) agent.SessionOptions {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.options) == 0 {
		t.Fatal("provider received no session options")
	}
	return p.options[len(p.options)-1]
}

func TestAdditionalSandboxFoldersPersistAndDescribeMainFolderToModel(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	main := filepath.Join(root, "main")
	extra := filepath.Join(root, "shared")
	next := filepath.Join(root, "generated")
	for _, folder := range []string{main, extra, next} {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "shared-link")
	if err := os.Symlink(extra, alias); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingProvider{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: main, DefaultModel: "fake-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{
		CWD: main, AdditionalFolders: []string{alias, extra}, Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonicalMain, _ := filepath.EvalSymlinks(main)
	canonicalExtra, _ := filepath.EvalSymlinks(extra)
	if project.CWD != canonicalMain || len(project.AdditionalFolders) != 1 || project.AdditionalFolders[0] != canonicalExtra {
		t.Fatalf("project folders = main:%q additional:%q", project.CWD, project.AdditionalFolders)
	}
	opened := provider.lastOptions(t)
	for _, want := range []string{
		"<project_sandbox>",
		"main project folder is " + strconv.Quote(canonicalMain),
		"primary working directory",
		strconv.Quote(canonicalExtra),
		"do not replace the main project folder",
	} {
		if !strings.Contains(opened.Instructions, want) {
			t.Fatalf("session instructions missing %q:\n%s", want, opened.Instructions)
		}
	}
	if opened.WorkingDirectory != canonicalMain {
		t.Fatalf("provider working directory = %q, want main %q", opened.WorkingDirectory, canonicalMain)
	}

	updated, err := manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{AdditionalFolders: &[]string{next}})
	if err != nil {
		t.Fatal(err)
	}
	canonicalNext, _ := filepath.EvalSymlinks(next)
	if len(updated.AdditionalFolders) != 1 || updated.AdditionalFolders[0] != canonicalNext {
		t.Fatalf("updated folders = %q", updated.AdditionalFolders)
	}
	reopened := provider.lastOptions(t)
	if !strings.Contains(reopened.Instructions, strconv.Quote(canonicalNext)) || strings.Contains(reopened.Instructions, strconv.Quote(canonicalExtra)) {
		t.Fatalf("reopened instructions did not replace folder set:\n%s", reopened.Instructions)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	restoredProvider := &recordingProvider{}
	restoredManager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: restoredProvider, DefaultCWD: main, DefaultModel: "fake-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restoredManager.Close()
	state, err := restoredManager.State(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Project.AdditionalFolders) != 1 || state.Project.AdditionalFolders[0] != canonicalNext {
		t.Fatalf("restored folders = %q", state.Project.AdditionalFolders)
	}
	if !strings.Contains(restoredProvider.lastOptions(t).Instructions, strconv.Quote(canonicalNext)) {
		t.Fatal("restored session did not receive persisted sandbox folder instructions")
	}
}

func TestAdditionalSandboxFolderValidation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	main := filepath.Join(root, "main")
	if err := os.MkdirAll(main, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{Store: store, Provider: &fakeProvider{}, DefaultCWD: main})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	for _, folders := range [][]string{{"relative/folder"}, {string(filepath.Separator)}} {
		if _, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: main, AdditionalFolders: folders}); err == nil {
			t.Fatalf("accepted unsafe additional folders %q", folders)
		}
	}
	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{Name: "pathless"})
	if err != nil {
		t.Fatal(err)
	}
	extra := t.TempDir()
	if _, err := manager.UpdateSettings(ctx, chat.ID, daemon.SettingsUpdate{AdditionalFolders: &[]string{extra}}); err == nil {
		t.Fatal("standalone chat accepted additional sandbox folders")
	}
}
