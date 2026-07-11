package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/threadstore"
)

func projectLaunchers(
	ctx context.Context,
	manager *Manager,
	settings *configuration.Store,
	projectID string,
) (threadstore.Project, []configuration.ProjectLauncher, error) {
	project, resolved, err := resolvedProjectLaunchers(ctx, manager, settings, projectID)
	if err != nil {
		return threadstore.Project{}, nil, err
	}
	launchers := make([]configuration.ProjectLauncher, 0, len(resolved))
	for _, entry := range resolved {
		launchers = append(launchers, entry.launcher)
	}
	return project, launchers, nil
}

type resolvedProjectLauncher struct {
	launcher configuration.ProjectLauncher
	script   string
}

func resolvedProjectLaunchers(
	ctx context.Context,
	manager *Manager,
	settings *configuration.Store,
	projectID string,
) (threadstore.Project, []resolvedProjectLauncher, error) {
	if manager == nil {
		return threadstore.Project{}, nil, errors.New("daemon: daemon is not initialized")
	}
	project, err := manager.Project(ctx, strings.TrimSpace(projectID))
	if err != nil || project.IsSubagent() || project.ResourceKind() != threadstore.KindProject {
		return threadstore.Project{}, nil, errors.New("daemon: launchers require a top-level project")
	}
	if err := validateProjectLauncherFolder(project); err != nil {
		return threadstore.Project{}, nil, err
	}
	var launchers []configuration.ProjectLauncher
	if settings == nil {
		launchers = configuration.DefaultProjectLaunchers()
	} else {
		effective, _, err := settings.RuntimeSettings(ctx, configScopeID(project))
		if err != nil {
			return threadstore.Project{}, nil, err
		}
		launchers = configuration.ResolveProjectLaunchers(effective.Launchers)
	}
	resolved := make([]resolvedProjectLauncher, 0, len(launchers)+8)
	ids := make(map[string]bool, len(launchers)+8)
	for _, launcher := range launchers {
		ids[launcher.ID] = true
		resolved = append(resolved, resolvedProjectLauncher{launcher: launcher})
	}
	folder := project.CWD
	var environments []ProjectEnvironment
	if project.Worktree != nil && project.Worktree.SourceCWD != "" {
		folder = project.Worktree.SourceCWD
		if project.Worktree.EnvironmentID != "" {
			environment, err := LoadProjectEnvironment(ctx, folder, project.Worktree.EnvironmentID)
			if err == nil {
				environments = []ProjectEnvironment{environment}
			}
		}
	} else {
		environments, _ = ListProjectEnvironments(ctx, folder)
	}
	platform := currentEnvironmentPlatform()
	for _, environment := range environments {
		for _, action := range environment.Actions {
			if action.Platform != "" && action.Platform != platform {
				continue
			}
			if ids[action.ID] {
				return threadstore.Project{}, nil, fmt.Errorf("daemon: duplicate project launcher id %q", action.ID)
			}
			ids[action.ID] = true
			resolved = append(resolved, resolvedProjectLauncher{
				launcher: configuration.ProjectLauncher{
					ID: action.ID, Label: environment.Name + " · " + action.Name,
					Kind: configuration.LauncherTerminal, Icon: action.Icon,
				},
				script: action.Command,
			})
		}
	}
	return project, resolved, nil
}

func configuredProjectLauncher(
	ctx context.Context,
	manager *Manager,
	settings *configuration.Store,
	projectID string,
	launcherID string,
) (threadstore.Project, resolvedProjectLauncher, error) {
	project, launchers, err := resolvedProjectLaunchers(ctx, manager, settings, projectID)
	if err != nil {
		return threadstore.Project{}, resolvedProjectLauncher{}, err
	}
	launcherID = strings.TrimSpace(launcherID)
	if launcherID == "" {
		return threadstore.Project{}, resolvedProjectLauncher{}, errors.New("daemon: launcher id is required")
	}
	for _, launcher := range launchers {
		if launcher.launcher.ID == launcherID {
			return project, launcher, nil
		}
	}
	return threadstore.Project{}, resolvedProjectLauncher{}, fmt.Errorf("daemon: unknown project launcher %q", launcherID)
}

func validateProjectLauncherFolder(project threadstore.Project) error {
	if strings.TrimSpace(project.CWD) == "" {
		return errors.New("daemon: project folder is unavailable")
	}
	info, err := os.Stat(project.CWD)
	if err != nil || !info.IsDir() {
		return errors.New("daemon: project folder is unavailable")
	}
	return nil
}

func projectTerminalCommand(ctx context.Context, project threadstore.Project, resolved resolvedProjectLauncher) (*exec.Cmd, error) {
	launcher := resolved.launcher
	if project.ResourceKind() != threadstore.KindProject || project.IsSubagent() {
		return nil, errors.New("daemon: terminal requires a top-level project")
	}
	if err := validateProjectLauncherFolder(project); err != nil {
		return nil, err
	}
	if launcher.Kind != configuration.LauncherTerminal {
		return nil, fmt.Errorf("daemon: launcher %q is not a terminal application", launcher.ID)
	}
	var executable string
	var arguments []string
	var err error
	if resolved.script != "" {
		if runtime.GOOS == "windows" {
			executable, err = exec.LookPath("powershell.exe")
			arguments = []string{"-NoProfile", "-NonInteractive", "-Command", resolved.script}
		} else {
			executable, err = exec.LookPath("bash")
			arguments = []string{"-c", resolved.script}
		}
	} else {
		executable, arguments, err = projectLauncherCommandLine(launcher)
	}
	if err != nil {
		return nil, fmt.Errorf("daemon: resolve launcher %q command: %w", launcher.ID, err)
	}
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = project.CWD
	environment := terminalEnvironment(os.Environ(), project.ID)
	if project.Worktree != nil {
		environment = environmentWithOverrides(environment, map[string]string{
			"CODEX_SOURCE_TREE_PATH": project.Worktree.SourceCWD,
			"CODEX_WORKTREE_PATH":    project.Worktree.Path,
		})
	}
	command.Env = environment
	return command, nil
}

func projectLauncherCommandLine(launcher configuration.ProjectLauncher) (string, []string, error) {
	if strings.TrimSpace(launcher.Command) == "" {
		if launcher.Kind != configuration.LauncherTerminal {
			return "", nil, fmt.Errorf("daemon: launcher %q command is required", launcher.ID)
		}
		executable := os.Getenv("SHELL")
		arguments := []string{"-l"}
		if executable == "" || !filepath.IsAbs(executable) {
			if runtime.GOOS == "windows" {
				executable = os.Getenv("COMSPEC")
				arguments = nil
			} else {
				executable = "/bin/sh"
			}
		}
		if executable == "" {
			return "", nil, errors.New("daemon: login shell is unavailable")
		}
		return executable, arguments, nil
	}
	executable, err := exec.LookPath(strings.TrimSpace(launcher.Command))
	if err != nil {
		return "", nil, fmt.Errorf("daemon: launcher %q command %q is not installed or not on PATH", launcher.ID, launcher.Command)
	}
	return executable, append([]string(nil), launcher.Args...), nil
}

func launchProjectDesktopApplication(
	ctx context.Context,
	manager *Manager,
	settings *configuration.Store,
	projectID string,
	launcherID string,
) (configuration.ProjectLauncher, error) {
	project, resolved, err := configuredProjectLauncher(ctx, manager, settings, projectID, launcherID)
	if err != nil {
		return configuration.ProjectLauncher{}, err
	}
	launcher := resolved.launcher
	if launcher.Kind != configuration.LauncherDesktop {
		return configuration.ProjectLauncher{}, fmt.Errorf("daemon: launcher %q is not a desktop application", launcher.ID)
	}
	executable, arguments, err := projectLauncherCommandLine(launcher)
	if err != nil {
		return configuration.ProjectLauncher{}, err
	}
	// Desktop applications deliberately outlive the requesting WebSocket. The
	// configured command is executed directly; no browser-provided argument is
	// ever interpreted by a shell.
	command := exec.Command(executable, arguments...)
	command.Dir = project.CWD
	command.Env = projectApplicationEnvironment(os.Environ(), project.ID)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return configuration.ProjectLauncher{}, fmt.Errorf("daemon: launch %q: %w", launcher.ID, err)
	}
	go func() { _ = command.Wait() }()
	return launcher, nil
}

func projectApplicationEnvironment(base []string, projectID string) []string {
	environment := make([]string, 0, len(base)+2)
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.EqualFold(name, "DIRE_AGENT_PROJECT_ID") || strings.EqualFold(name, "GOAGENT_PROJECT_ID")) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"DIRE_AGENT_PROJECT_ID="+projectID,
		"GOAGENT_PROJECT_ID="+projectID,
	)
}
