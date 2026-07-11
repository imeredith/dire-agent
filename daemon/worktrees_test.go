package daemon_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestManagedWorktreeCreatesDetachedNestedProjectRunsSetupAndPreservesCheckout(t *testing.T) {
	requireGitAndBash(t)
	ctx := context.Background()
	repository := initializeWorktreeRepository(t)
	sourceFolder := filepath.Join(repository, "apps", "demo")
	platformScript := "printf 'platform\\n%s\\n%s\\n%s\\n' \"$PWD\" \"$CODEX_SOURCE_TREE_PATH\" \"$CODEX_WORKTREE_PATH\" > setup-result.txt"
	setup := daemon.EnvironmentScript{Script: "printf 'default' > setup-result.txt"}
	switch runtime.GOOS {
	case "darwin":
		setup.Darwin = &daemon.EnvironmentPlatformScript{Script: platformScript}
	case "linux":
		setup.Linux = &daemon.EnvironmentPlatformScript{Script: platformScript}
	default:
		t.Skip("managed worktree setup integration is supported on macOS and Linux")
	}
	if _, err := daemon.PutProjectEnvironment(ctx, sourceFolder, daemon.ProjectEnvironment{
		ID: "environment.toml", Version: 1, Name: "Development", Setup: setup,
		Actions: []daemon.EnvironmentAction{{Name: "Run", Icon: "run", Command: "echo run"}},
	}, ""); err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	store, err := threadstore.New(filepath.Join(stateRoot, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingProvider{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: sourceFolder,
		WorktreeRoot: filepath.Join(stateRoot, "worktrees"),
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: sourceFolder})
	if err != nil {
		t.Fatal(err)
	}
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{
		Worktree: &daemon.CreateWorktreeOptions{
			BaseRef: "main", EnvironmentID: "environment.toml", SourceProjectID: source.ID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Worktree == nil {
		t.Fatal("managed project omitted worktree metadata")
	}
	if project.SettingsID != source.ID {
		t.Fatalf("settings id = %q, want source %q", project.SettingsID, source.ID)
	}
	if project.Worktree.ProjectRelativePath != filepath.Join("apps", "demo") {
		t.Fatalf("relative path = %q", project.Worktree.ProjectRelativePath)
	}
	if project.CWD != filepath.Join(project.Worktree.Path, "apps", "demo") {
		t.Fatalf("project cwd = %q, worktree = %#v", project.CWD, project.Worktree)
	}
	if provider.lastOptions(t).WorkingDirectory != project.CWD {
		t.Fatalf("provider cwd = %q, want %q", provider.lastOptions(t).WorkingDirectory, project.CWD)
	}
	if branch := runTestGit(t, project.Worktree.Path, "rev-parse", "--abbrev-ref", "HEAD"); branch != "HEAD" {
		t.Fatalf("managed checkout branch = %q, want detached HEAD", branch)
	}
	if head := runTestGit(t, project.Worktree.Path, "rev-parse", "HEAD"); head != project.Worktree.BaseCommit {
		t.Fatalf("checkout HEAD = %q, metadata = %q", head, project.Worktree.BaseCommit)
	}
	setupResult, err := os.ReadFile(filepath.Join(project.CWD, "setup-result.txt"))
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []string{"platform", project.CWD, project.Worktree.SourceCWD, project.Worktree.Path}
	if got := strings.Split(strings.TrimSpace(string(setupResult)), "\n"); strings.Join(got, "\n") != strings.Join(wantLines, "\n") {
		t.Fatalf("setup result = %q, want %q", got, wantLines)
	}
	if _, err := os.Stat(filepath.Join(sourceFolder, "setup-result.txt")); !os.IsNotExist(err) {
		t.Fatalf("setup mutated source checkout: %v", err)
	}
	worktreePath := project.Worktree.Path
	projectID := project.ID
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: sourceFolder,
		WorktreeRoot: filepath.Join(stateRoot, "worktrees"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredState, err := restored.State(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredState.Project.Worktree == nil || restoredState.Project.Worktree.BaseCommit != project.Worktree.BaseCommit {
		t.Fatalf("restored worktree = %#v", restoredState.Project.Worktree)
	}
	if err := restored.DeleteProject(ctx, projectID); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(worktreePath); err != nil || !info.IsDir() {
		t.Fatalf("history deletion removed published worktree: info=%v err=%v", info, err)
	}
	if output := runTestGit(t, repository, "worktree", "list", "--porcelain"); !strings.Contains(output, worktreePath) {
		t.Fatalf("published checkout was unregistered after history deletion:\n%s", output)
	}
}

func TestManagedWorktreeSetupFailureRollsBackCheckoutAndHistory(t *testing.T) {
	requireGitAndBash(t)
	ctx := context.Background()
	repository := initializeWorktreeRepository(t)
	sourceFolder := filepath.Join(repository, "apps", "demo")
	if _, err := daemon.PutProjectEnvironment(ctx, sourceFolder, daemon.ProjectEnvironment{
		ID: "broken.toml", Version: 1, Name: "Broken",
		Setup: daemon.EnvironmentScript{Script: "echo partial > partial.txt; exit 9"},
	}, ""); err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	store, err := threadstore.New(filepath.Join(stateRoot, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	worktreeRoot := filepath.Join(stateRoot, "worktrees")
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: sourceFolder, WorktreeRoot: worktreeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.CreateProject(ctx, daemon.CreateProjectOptions{
		CWD:      sourceFolder,
		Worktree: &daemon.CreateWorktreeOptions{BaseRef: "main", EnvironmentID: "broken.toml"},
	})
	if err == nil || !strings.Contains(err.Error(), "worktree setup failed") {
		t.Fatalf("setup failure = %v", err)
	}
	entries, err := os.ReadDir(worktreeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unpublished worktree paths remain: %v", entries)
	}
	resources, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("failed creation published history: %#v", resources)
	}
	if output := runTestGit(t, repository, "worktree", "list", "--porcelain"); strings.Contains(output, worktreeRoot) {
		t.Fatalf("failed checkout remains registered:\n%s", output)
	}
}

func TestWorkspaceInspectionHandlesNonGitAndMalformedUnselectedEnvironment(t *testing.T) {
	requireGitAndBash(t)
	ctx := context.Background()
	nonGit := t.TempDir()
	store, err := threadstore.New(filepath.Join(t.TempDir(), "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{Store: store, Provider: &fakeProvider{}, DefaultCWD: nonGit})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	inspection, err := manager.InspectProjectWorkspace(ctx, "", nonGit)
	if err != nil || inspection.GitRepository {
		t.Fatalf("non-Git inspection = %#v, err = %v", inspection, err)
	}
	if _, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: nonGit, Worktree: &daemon.CreateWorktreeOptions{}}); err == nil || !strings.Contains(err.Error(), "require a Git repository") {
		t.Fatalf("non-Git worktree error = %v", err)
	}

	repository := initializeWorktreeRepository(t)
	sourceFolder := filepath.Join(repository, "apps", "demo")
	directory := filepath.Join(sourceFolder, ".codex", "environments")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "malformed.toml"), []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: sourceFolder, Worktree: &daemon.CreateWorktreeOptions{BaseRef: "main"}})
	if err != nil {
		t.Fatalf("unselected malformed environment blocked worktree: %v", err)
	}
	if project.Worktree == nil {
		t.Fatal("worktree metadata missing")
	}
}

func TestManagerRejectsPermissiveWorktreeRootWithoutChangingPermissions(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := threadstore.New(filepath.Join(stateRoot, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	worktreeRoot := filepath.Join(stateRoot, "shared-worktrees")
	if err := os.Mkdir(worktreeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(worktreeRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: stateRoot, WorktreeRoot: worktreeRoot,
	}); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("NewManager error = %v, want a permissions error", err)
	}
	info, err := os.Stat(worktreeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o777 {
		t.Fatalf("NewManager changed existing worktree root permissions to %#o", got)
	}
}

func initializeWorktreeRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "apps", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "apps", "demo", "tracked.txt"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "init", "-q")
	runTestGit(t, repository, "config", "user.email", "test@example.com")
	runTestGit(t, repository, "config", "user.name", "Dire Agent Test")
	runTestGit(t, repository, "add", ".")
	runTestGit(t, repository, "commit", "-qm", "initial")
	runTestGit(t, repository, "branch", "-M", "main")
	return repository
}

func requireGitAndBash(t *testing.T) {
	t.Helper()
	for _, command := range []string{"git", "bash"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is unavailable", command)
		}
	}
}

func runTestGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
