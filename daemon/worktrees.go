package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dire-kiwi/dire-agent/threadstore"
)

const (
	worktreeSetupTimeout = 10 * time.Minute
	maxWorktreeOutput    = 1 << 20
)

func (m *Manager) InspectProjectWorkspace(ctx context.Context, projectID, folder string) (ProjectWorkspaceInspection, error) {
	folder, err := m.resolveEnvironmentFolder(ctx, projectID, folder)
	if err != nil {
		return ProjectWorkspaceInspection{}, err
	}
	inspection, err := inspectGitWorkspace(ctx, folder)
	if err != nil {
		return ProjectWorkspaceInspection{}, err
	}
	inspection.Environments, _ = ListProjectEnvironments(ctx, folder)
	return inspection, nil
}

func inspectGitWorkspace(ctx context.Context, folder string) (ProjectWorkspaceInspection, error) {
	inspection := ProjectWorkspaceInspection{
		Folder: folder, Branches: []string{}, Environments: []ProjectEnvironment{},
	}
	repositoryRoot, err := gitOutput(ctx, folder, "rev-parse", "--show-toplevel")
	if err != nil {
		if isGitExitError(err) {
			return inspection, nil
		}
		return ProjectWorkspaceInspection{}, err
	}
	repositoryRoot, err = canonicalProjectFolder(repositoryRoot)
	if err != nil {
		return ProjectWorkspaceInspection{}, fmt.Errorf("daemon: inspect Git repository: %w", err)
	}
	relative, err := filepath.Rel(repositoryRoot, folder)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ProjectWorkspaceInspection{}, errors.New("daemon: project folder is outside its Git repository")
	}
	if relative == "." {
		relative = ""
	}
	inspection.GitRepository = true
	inspection.RepositoryRoot = repositoryRoot
	inspection.ProjectRelativePath = relative
	inspection.Head, err = gitOutput(ctx, repositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return ProjectWorkspaceInspection{}, fmt.Errorf("daemon: resolve Git HEAD: %w", err)
	}
	inspection.CurrentBranch, _ = gitOutput(ctx, repositoryRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	refs, err := gitOutput(ctx, repositoryRoot, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return ProjectWorkspaceInspection{}, fmt.Errorf("daemon: list Git branches: %w", err)
	}
	seen := make(map[string]bool)
	for _, ref := range strings.Split(refs, "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasSuffix(ref, "/HEAD") || seen[ref] {
			continue
		}
		seen[ref] = true
		inspection.Branches = append(inspection.Branches, ref)
	}
	sort.Strings(inspection.Branches)
	return inspection, nil
}

func (m *Manager) ProjectEnvironments(ctx context.Context, projectID, folder string) ([]ProjectEnvironment, error) {
	folder, err := m.resolveEnvironmentFolder(ctx, projectID, folder)
	if err != nil {
		return nil, err
	}
	return ListProjectEnvironments(ctx, folder)
}

func (m *Manager) PutProjectEnvironment(ctx context.Context, projectID, folder string, environment ProjectEnvironment, expectedHash string) (ProjectEnvironment, error) {
	folder, err := m.resolveEnvironmentFolder(ctx, projectID, folder)
	if err != nil {
		return ProjectEnvironment{}, err
	}
	return PutProjectEnvironment(ctx, folder, environment, expectedHash)
}

func (m *Manager) DeleteProjectEnvironment(ctx context.Context, projectID, folder, environmentID, expectedHash string) error {
	folder, err := m.resolveEnvironmentFolder(ctx, projectID, folder)
	if err != nil {
		return err
	}
	return DeleteProjectEnvironment(ctx, folder, environmentID, expectedHash)
}

func (m *Manager) resolveEnvironmentFolder(ctx context.Context, projectID, folder string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		if strings.TrimSpace(folder) == "" {
			return "", errors.New("daemon: project_id or folder is required")
		}
		return canonicalProjectFolder(folder)
	}
	project, err := m.Project(ctx, projectID)
	if err != nil || project.IsSubagent() {
		return "", errors.New("daemon: environments require a top-level project")
	}
	resolved := project.CWD
	if project.Worktree != nil && project.Worktree.SourceCWD != "" {
		resolved = project.Worktree.SourceCWD
	}
	resolved, err = canonicalProjectFolder(resolved)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(folder) != "" {
		provided, err := canonicalProjectFolder(folder)
		if err != nil {
			return "", err
		}
		if provided != resolved {
			return "", errors.New("daemon: folder does not match the project source folder")
		}
	}
	return resolved, nil
}

func (m *Manager) createManagedWorktree(
	ctx context.Context,
	id string,
	sourceFolder string,
	options CreateWorktreeOptions,
) (string, *threadstore.WorktreeInfo, func() error, error) {
	inspection, err := inspectGitWorkspace(ctx, sourceFolder)
	if err != nil {
		return "", nil, nil, err
	}
	if !inspection.GitRepository {
		return "", nil, nil, errors.New("daemon: worktrees require a Git repository")
	}
	if pathWithin(inspection.RepositoryRoot, m.config.WorktreeRoot) {
		return "", nil, nil, errors.New("daemon: worktree root must be outside the source repository")
	}
	baseRef := strings.TrimSpace(options.BaseRef)
	if baseRef == "" {
		baseRef = "HEAD"
	}
	baseCommit, err := gitOutput(ctx, inspection.RepositoryRoot,
		"rev-parse", "--verify", "--end-of-options", baseRef+"^{commit}")
	if err != nil {
		return "", nil, nil, fmt.Errorf("daemon: resolve worktree base ref %q: %w", baseRef, err)
	}
	var environment *ProjectEnvironment
	if options.EnvironmentID != "" {
		loaded, err := LoadProjectEnvironment(ctx, sourceFolder, options.EnvironmentID)
		if err != nil {
			return "", nil, nil, err
		}
		environment = &loaded
	}
	if err := os.MkdirAll(m.config.WorktreeRoot, 0o700); err != nil {
		return "", nil, nil, fmt.Errorf("daemon: create worktree root: %w", err)
	}
	worktreePath := filepath.Join(m.config.WorktreeRoot, id)
	if _, err := os.Lstat(worktreePath); err == nil {
		return "", nil, nil, fmt.Errorf("daemon: managed worktree path already exists: %s", worktreePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, nil, fmt.Errorf("daemon: inspect managed worktree path: %w", err)
	}
	m.worktreeMu.Lock()
	_, addErr := gitOutput(ctx, inspection.RepositoryRoot, "worktree", "add", "--detach", worktreePath, baseCommit)
	m.worktreeMu.Unlock()
	if addErr != nil {
		rollbackErr := m.rollbackManagedWorktree(inspection.RepositoryRoot, worktreePath)
		return "", nil, nil, errors.Join(fmt.Errorf("daemon: create managed worktree: %w", addErr), rollbackErr)
	}
	var rollbackOnce sync.Once
	var rollbackErr error
	rollback := func() error {
		rollbackOnce.Do(func() {
			rollbackErr = m.rollbackManagedWorktree(inspection.RepositoryRoot, worktreePath)
		})
		return rollbackErr
	}
	projectCWD := worktreePath
	if inspection.ProjectRelativePath != "" {
		projectCWD = filepath.Join(projectCWD, inspection.ProjectRelativePath)
	}
	projectCWD, err = canonicalProjectFolder(projectCWD)
	if err != nil {
		return "", nil, nil, errors.Join(fmt.Errorf("daemon: resolve managed project folder: %w", err), rollback())
	}
	if environment != nil {
		script := environmentScriptForPlatform(environment.Setup, currentEnvironmentPlatform())
		if strings.TrimSpace(script) != "" {
			if err := runWorktreeSetup(ctx, projectCWD, sourceFolder, worktreePath, script); err != nil {
				return "", nil, nil, errors.Join(err, rollback())
			}
		}
	}
	info := &threadstore.WorktreeInfo{
		SourceCWD: sourceFolder, SourceRepository: inspection.RepositoryRoot,
		Path: worktreePath, ProjectRelativePath: inspection.ProjectRelativePath,
		BaseRef: baseRef, BaseCommit: baseCommit, EnvironmentID: options.EnvironmentID,
	}
	return projectCWD, info, rollback, nil
}

func (m *Manager) rollbackManagedWorktree(repositoryRoot, worktreePath string) error {
	m.worktreeMu.Lock()
	defer m.worktreeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, removeErr := gitOutput(ctx, repositoryRoot, "worktree", "remove", "--force", worktreePath)
	var cleanupErr error
	if removeErr != nil {
		if m.ownsManagedWorktreePath(worktreePath) {
			cleanupErr = os.RemoveAll(worktreePath)
		} else if _, statErr := os.Lstat(worktreePath); statErr == nil {
			cleanupErr = errors.New("daemon: refusing to remove an unverified managed worktree path")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			cleanupErr = statErr
		}
	}
	_, pruneErr := gitOutput(ctx, repositoryRoot, "worktree", "prune")
	_, statErr := os.Lstat(worktreePath)
	if errors.Is(statErr, os.ErrNotExist) && pruneErr == nil && cleanupErr == nil {
		return nil
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		cleanupErr = errors.Join(cleanupErr, statErr)
	} else if statErr == nil {
		cleanupErr = errors.Join(cleanupErr, errors.New("daemon: managed worktree path remains after rollback"))
	}
	return errors.Join(removeErr, cleanupErr, pruneErr)
}

func (m *Manager) ownsManagedWorktreePath(path string) bool {
	root, err := filepath.EvalSymlinks(m.config.WorktreeRoot)
	if err != nil || root != m.config.WorktreeRoot || filepath.Dir(path) != root {
		return false
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	pathInfo, err := os.Lstat(path)
	return err == nil && pathInfo.IsDir() && pathInfo.Mode()&os.ModeSymlink == 0
}

func runWorktreeSetup(ctx context.Context, cwd, sourceFolder, worktreePath, script string) error {
	setupContext, cancel := context.WithTimeout(ctx, worktreeSetupTimeout)
	defer cancel()
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		executable, err := exec.LookPath("powershell.exe")
		if err != nil {
			return errors.New("daemon: worktree setup requires PowerShell on Windows")
		}
		command = exec.CommandContext(setupContext, executable, "-NoProfile", "-NonInteractive", "-Command", script)
	} else {
		executable, err := exec.LookPath("bash")
		if err != nil {
			return errors.New("daemon: worktree setup requires bash")
		}
		command = exec.CommandContext(setupContext, executable, "-c", script)
	}
	command.Dir = cwd
	configureSetupCommand(command)
	command.Env = environmentWithOverrides(os.Environ(), map[string]string{
		"CODEX_SOURCE_TREE_PATH": sourceFolder,
		"CODEX_WORKTREE_PATH":    worktreePath,
	})
	output := &boundedCommandOutput{limit: maxWorktreeOutput}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if errors.Is(setupContext.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("daemon: worktree setup timed out after %s: %s", worktreeSetupTimeout, output.String())
	}
	if err != nil {
		return fmt.Errorf("daemon: worktree setup failed: %w: %s", err, output.String())
	}
	return nil
}

func gitOutput(ctx context.Context, directory string, arguments ...string) (string, error) {
	executable, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("daemon: git is not installed or not on PATH")
	}
	args := append([]string{"-C", directory}, arguments...)
	command := exec.CommandContext(ctx, executable, args...)
	configureSetupCommand(command)
	output := &boundedCommandOutput{limit: maxWorktreeOutput}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(output.String()))
	}
	return strings.TrimSpace(output.String()), nil
}

func isGitExitError(err error) bool {
	var exitError *exec.ExitError
	return errors.As(err, &exitError)
}

func environmentWithOverrides(base []string, overrides map[string]string) []string {
	replaced := make(map[string]bool, len(overrides))
	for key := range overrides {
		replaced[strings.ToUpper(key)] = true
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found && replaced[strings.ToUpper(name)] {
			continue
		}
		result = append(result, entry)
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, key+"="+overrides[key])
	}
	return result
}

type boundedCommandOutput struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedCommandOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(data)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
			b.truncated = true
		}
		_, _ = b.buffer.Write(data)
	} else if original != 0 {
		b.truncated = true
	}
	return original, nil
}

func (b *boundedCommandOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	value := strings.TrimSpace(b.buffer.String())
	if b.truncated {
		value += "\n[output truncated]"
	}
	return value
}
