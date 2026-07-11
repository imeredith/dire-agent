package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultDarwinSandboxExecutable = "/usr/bin/sandbox-exec"
	defaultLinuxSandboxExecutable  = "/usr/bin/bwrap"
	defaultSandboxShell            = "/bin/sh"
)

// BuiltinOptions controls the executor used by the bash built-in. The zero
// value is the secure production configuration. The fields are exported to
// permit a fake sandbox executable and macOS profile builder in hermetic tests.
// Supplying a custom executable or profile is security-sensitive and should
// not be necessary in normal use.
type BuiltinOptions struct {
	SandboxExecutable string
	SandboxProfile    func(workspace string, additionalFolders []string) (string, error)
	AdditionalFolders []string
}

// shellExecutor is deliberately narrower than exec.Cmd. Keeping command
// construction behind this interface makes it impossible for bashTool to
// accidentally bypass the platform sandbox in a future code path.
type shellExecutor interface {
	Run(ctx context.Context, dir, command string, output io.Writer) error
}

type sandboxExecutor struct {
	executable string
	args       []string
}

func newSandboxExecutor(workspace string, options BuiltinOptions) (*sandboxExecutor, error) {
	// SandboxProfile is a Darwin-specific test seam. Preserve its historical
	// sandbox-exec argv contract on every host so existing embedders can use a
	// fake wrapper without depending on the host operating system.
	if options.SandboxProfile != nil {
		return newDarwinSandboxExecutor(workspace, options)
	}

	executable, args, err := wrapSandboxedProcessForPlatform(runtime.GOOS, options.SandboxExecutable, ProcessSandbox{
		Workspace:            workspace,
		WorkingDirectory:     workspace,
		Command:              defaultSandboxShell,
		AdditionalWritePaths: options.AdditionalFolders,
	})
	if err != nil {
		return nil, fmt.Errorf("tools: bash sandbox unavailable; refusing to run unsandboxed: %w", err)
	}
	return &sandboxExecutor{executable: executable, args: args}, nil
}

func newDarwinSandboxExecutor(workspace string, options BuiltinOptions) (*sandboxExecutor, error) {
	executable := options.SandboxExecutable
	if executable == "" {
		executable = defaultDarwinSandboxExecutable
	}
	resolvedExecutable, err := validateExecutable(executable)
	if err != nil {
		return nil, fmt.Errorf("tools: bash sandbox unavailable; refusing to run unsandboxed: %w", err)
	}
	profile, err := options.SandboxProfile(workspace, append([]string(nil), options.AdditionalFolders...))
	if err != nil {
		return nil, fmt.Errorf("tools: build bash sandbox profile: %w", err)
	}
	if strings.TrimSpace(profile) == "" {
		return nil, errors.New("tools: bash sandbox profile is empty; refusing to run unsandboxed")
	}
	return &sandboxExecutor{
		executable: resolvedExecutable,
		args:       []string{"-p", profile, defaultSandboxShell},
	}, nil
}

func validateExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("sandbox executable must be an absolute path: %q", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox executable %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat sandbox executable %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("sandbox executable is not an executable regular file: %q", resolved)
	}
	return resolved, nil
}

func (e *sandboxExecutor) Run(ctx context.Context, dir, command string, output io.Writer) error {
	// command is a distinct argv value after -c. It is never interpolated into
	// the wrapper command, so shell syntax cannot alter the sandbox flags.
	// A non-login shell also avoids reading a user profile outside the project.
	args := append([]string(nil), e.args...)
	args = append(args, "-c", command)
	cmd := exec.CommandContext(ctx, e.executable, args...)
	cmd.Dir = dir
	cmd.Env = SanitizeSandboxEnvironment(os.Environ())
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func defaultSandboxProfile(workspace string) (string, error) {
	return sandboxProfile(workspace, nil, false)
}

func sandboxProfile(workspace string, extraReadPaths []string, allowNetwork bool) (string, error) {
	return sandboxProfileWithWritePaths(workspace, extraReadPaths, nil, allowNetwork)
}

func sandboxProfileWithWritePaths(workspace string, extraReadPaths, extraWritePaths []string, allowNetwork bool) (string, error) {
	workspace, err := canonicalDirectory(workspace)
	if err != nil {
		return "", err
	}
	writeRoots := []string{workspace}
	for _, path := range extraWritePaths {
		resolved, resolveErr := canonicalDirectory(path)
		if resolveErr != nil {
			return "", fmt.Errorf("resolve additional sandbox folder %q: %w", path, resolveErr)
		}
		if filepath.Dir(resolved) == resolved {
			return "", errors.New("filesystem root cannot be an additional sandbox folder")
		}
		writeRoots = append(writeRoots, resolved)
	}
	writeRoots = normalizedPaths(writeRoots)

	// These paths contain platform executables, dynamic libraries, SDKs, and
	// package-manager binaries. They are read-only in the generated profile.
	readPaths := append([]string{
		"/Applications/Xcode.app",
		"/Library",
		"/System",
		"/bin",
		"/opt",
		"/sbin",
		"/usr",
		"/private/etc",
		"/private/var/db/timezone",
		"/private/var/select",
	}, writeRoots...)
	readPaths = append(readPaths, extraReadPaths...)

	// macOS tools need a temporary directory for compilers, lock files, and
	// atomic renames. os.TempDir is normally a per-user Darwin temp directory;
	// /private/tmp and /private/var/tmp cover tools that hard-code POSIX paths.
	tempPaths := []string{os.TempDir(), "/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp"}
	tempPaths = normalizedPaths(tempPaths)
	readPaths = normalizedPaths(append(readPaths, tempPaths...))
	writePaths := normalizedPaths(append(writeRoots, tempPaths...))

	var profile strings.Builder
	profile.WriteString("(version 1)\n")
	profile.WriteString("(deny default)\n")
	// Apple's base profile grants the runtime services required by current
	// macOS binaries (dyld, sysctls, and tightly-scoped Mach services). Our
	// explicit network denial and file rules below remain the confinement
	// boundary for commands and their descendants.
	profile.WriteString("(import \"system.sb\")\n")
	if !allowNetwork {
		profile.WriteString("(deny network*)\n")
	}
	// system.sb has a handful of platform write grants (for example core
	// dumps). Re-establish a deny baseline before the scoped grants below.
	profile.WriteString("(deny file-write*)\n")
	profile.WriteString("(allow process*)\n")
	profile.WriteString("(allow sysctl-read)\n")
	profile.WriteString("(allow file-read-metadata file-test-existence\n")
	profile.WriteString("  (literal \"/\")\n")
	for _, path := range readPaths {
		fmt.Fprintf(&profile, "  (path-ancestors %s)\n", strconv.Quote(path))
	}
	profile.WriteString(")\n")
	profile.WriteString("(allow file-read* file-test-existence\n")
	for _, path := range readPaths {
		fmt.Fprintf(&profile, "  (subpath %s)\n", strconv.Quote(path))
	}
	profile.WriteString("  (literal \"/dev/null\")\n")
	profile.WriteString("  (literal \"/dev/random\")\n")
	profile.WriteString("  (literal \"/dev/urandom\")\n")
	profile.WriteString("  (literal \"/dev/zero\")\n")
	profile.WriteString("  (subpath \"/dev/fd\")\n")
	profile.WriteString(")\n")
	profile.WriteString("(allow file-write*\n")
	for _, path := range writePaths {
		fmt.Fprintf(&profile, "  (subpath %s)\n", strconv.Quote(path))
	}
	profile.WriteString("  (literal \"/dev/null\")\n")
	profile.WriteString("  (literal \"/dev/zero\")\n")
	profile.WriteString("  (subpath \"/dev/fd\")\n")
	profile.WriteString(")\n")
	return profile.String(), nil
}

func canonicalDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project folder: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve project folder symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat project folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project folder is not a directory: %s", resolved)
	}
	return filepath.Clean(resolved), nil
}

func normalizedPaths(paths []string) []string {
	unique := make(map[string]struct{}, len(paths)*2)
	for _, path := range paths {
		if path == "" || strings.IndexByte(path, 0) >= 0 {
			continue
		}
		clean := filepath.Clean(path)
		unique[clean] = struct{}{}
		if resolved, err := filepath.EvalSymlinks(clean); err == nil {
			unique[filepath.Clean(resolved)] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
