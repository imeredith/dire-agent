package tools

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/imeredith/dire-agent/internal/sandboxenv"
)

// ProcessSandbox describes an argv-safe platform sandbox wrapper. Workspace,
// additional write paths, and temporary paths are writable; ExtraReadPaths
// remain read-only. WorkingDirectory defaults to Workspace.
type ProcessSandbox struct {
	Workspace            string
	WorkingDirectory     string
	Command              string
	Args                 []string
	ExtraReadPaths       []string
	AdditionalWritePaths []string
	AllowNetwork         bool
	// PrivateWorkspace creates Workspace inside the Linux sandbox instead of
	// bind-mounting its host contents. It is used for pathless local processes.
	PrivateWorkspace bool
}

// WrapSandboxedProcess returns a platform sandbox command and argv without
// invoking a shell. It uses sandbox-exec on macOS and Bubblewrap on Linux, and
// fails closed when the platform sandbox or workspace is unavailable. Callers
// must pass SanitizeSandboxEnvironment(os.Environ()) (plus safe overrides) to
// exec.Cmd.Env so loader controls cannot run before the wrapper confines them.
func WrapSandboxedProcess(options ProcessSandbox) (string, []string, error) {
	return wrapSandboxedProcessForPlatform(runtime.GOOS, "", options)
}

// SanitizeSandboxEnvironment removes dynamic-loader controls that would take
// effect before sandbox-exec or Bubblewrap establishes confinement.
func SanitizeSandboxEnvironment(environment []string) []string {
	return sandboxenv.Sanitize(environment)
}

func wrapSandboxedProcessForPlatform(platform, sandboxExecutable string, options ProcessSandbox) (string, []string, error) {
	workingDirectory := options.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = options.Workspace
	}
	command, err := resolveProcessCommand(options.Command, workingDirectory)
	if err != nil {
		return "", nil, fmt.Errorf("tools: resolve process command: %w", err)
	}

	switch platform {
	case "darwin":
		if sandboxExecutable == "" {
			sandboxExecutable = defaultDarwinSandboxExecutable
		}
		sandbox, err := validateExecutable(sandboxExecutable)
		if err != nil {
			return "", nil, fmt.Errorf("tools: process sandbox unavailable: %w", err)
		}
		reads := append([]string(nil), options.ExtraReadPaths...)
		reads = append(reads, command, workingDirectory)
		profile, err := sandboxProfileWithWritePaths(options.Workspace, reads, options.AdditionalWritePaths, options.AllowNetwork)
		if err != nil {
			return "", nil, fmt.Errorf("tools: build process sandbox profile: %w", err)
		}
		args := []string{"-p", profile, command}
		args = append(args, options.Args...)
		return sandbox, args, nil
	case "linux":
		if sandboxExecutable == "" {
			sandboxExecutable = defaultLinuxSandboxExecutable
		}
		sandbox, err := validateExecutable(sandboxExecutable)
		if err != nil {
			return "", nil, fmt.Errorf("tools: Linux process sandbox unavailable; install bubblewrap at %s: %w", defaultLinuxSandboxExecutable, err)
		}
		args, err := bubblewrapArgs(bubblewrapConfig{
			Workspace:            options.Workspace,
			WorkingDirectory:     workingDirectory,
			Command:              command,
			Args:                 options.Args,
			ExtraReadPaths:       options.ExtraReadPaths,
			AdditionalWritePaths: options.AdditionalWritePaths,
			AllowNetwork:         options.AllowNetwork,
			PrivateWorkspace:     options.PrivateWorkspace,
			TemporaryPaths:       []string{os.TempDir(), "/tmp", "/var/tmp"},
		})
		if err != nil {
			return "", nil, fmt.Errorf("tools: build Linux process sandbox: %w", err)
		}
		return sandbox, args, nil
	default:
		return "", nil, fmt.Errorf("tools: process sandbox unavailable on %s; supported platforms are macOS and Linux", platform)
	}
}

func resolveProcessCommand(command, workingDirectory string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("command is empty")
	}
	if strings.IndexByte(command, 0) >= 0 {
		return "", errors.New("command contains NUL")
	}
	var err error
	switch {
	case filepath.IsAbs(command):
		command = filepath.Clean(command)
	case strings.ContainsRune(command, filepath.Separator):
		if workingDirectory == "" {
			return "", errors.New("relative command with a path requires a working directory")
		}
		command, err = filepath.Abs(filepath.Join(workingDirectory, command))
	default:
		command, err = exec.LookPath(command)
		if err == nil {
			command, err = filepath.Abs(command)
		}
	}
	if err != nil {
		return "", err
	}
	info, err := os.Stat(command)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("command is not an executable regular file: %q", command)
	}
	return command, nil
}
