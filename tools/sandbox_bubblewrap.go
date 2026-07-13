package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// bubblewrapConfig is deliberately independent of runtime.GOOS so the Linux
// policy can be verified hermetically on every development host.
type bubblewrapConfig struct {
	Workspace            string
	WorkingDirectory     string
	Command              string
	Args                 []string
	ExtraReadPaths       []string
	AdditionalWritePaths []string
	AllowNetwork         bool
	PrivateWorkspace     bool
	SystemPaths          []string
	ResolverPaths        []string
	TemporaryPaths       []string
}

var defaultBubblewrapSystemPaths = []string{
	"/usr",
	"/etc",
	"/opt",
	"/bin",
	"/sbin",
	"/lib",
	"/lib32",
	"/lib64",
	"/libx32",
	"/nix/store",
	"/gnu/store",
	"/snap",
	"/run/current-system",
}

func bubblewrapArgs(config bubblewrapConfig) ([]string, error) {
	workspace, err := canonicalDirectory(config.Workspace)
	if err != nil {
		return nil, err
	}
	if isFilesystemRoot(workspace) {
		return nil, errors.New("filesystem root cannot be a Bubblewrap workspace")
	}
	workingDirectory := config.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = workspace
	}
	workingDirectory, err = canonicalDirectory(workingDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox working directory: %w", err)
	}

	var writeRoots []string
	if !config.PrivateWorkspace {
		writeRoots = append(writeRoots, workspace)
	}
	for _, path := range config.AdditionalWritePaths {
		resolved, resolveErr := canonicalDirectory(path)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve additional sandbox folder %q: %w", path, resolveErr)
		}
		if isFilesystemRoot(resolved) {
			return nil, errors.New("filesystem root cannot be an additional sandbox folder")
		}
		writeRoots = append(writeRoots, resolved)
	}
	writeRoots = uniqueOrderedPaths(writeRoots)

	temporaryPaths := config.TemporaryPaths
	if temporaryPaths == nil {
		temporaryPaths = []string{os.TempDir(), "/tmp", "/var/tmp"}
	}
	temporaryPaths, err = cleanAbsolutePaths(temporaryPaths, "temporary directory")
	if err != nil {
		return nil, err
	}
	for _, path := range temporaryPaths {
		if isFilesystemRoot(path) {
			return nil, errors.New("filesystem root cannot be a sandbox temporary directory")
		}
	}

	args := []string{
		"--unshare-user",
		"--unshare-ipc",
		"--unshare-pid",
		"--unshare-uts",
		"--cap-drop",
		"ALL",
	}
	if !config.AllowNetwork {
		args = append(args, "--unshare-net")
	}
	args = append(args, "--die-with-parent", "--new-session")

	systemPaths := config.SystemPaths
	if systemPaths == nil {
		systemPaths = defaultBubblewrapSystemPaths
	}
	potentialSystemRoots, err := cleanAbsolutePaths(systemPaths, "system read path")
	if err != nil {
		return nil, err
	}
	visibleReadRoots := make([]string, 0, len(potentialSystemRoots)+len(config.ExtraReadPaths)+2)
	for _, path := range potentialSystemRoots {
		var mounted bool
		args, mounted, err = appendBubblewrapSystemPath(args, path, potentialSystemRoots)
		if err != nil {
			return nil, err
		}
		if mounted {
			visibleReadRoots = append(visibleReadRoots, path)
		}
	}

	// Never expose the host's procfs or devices. Bubblewrap creates fresh
	// instances inside the PID/user namespace, plus private sticky temp mounts.
	args = append(args, "--proc", "/proc", "--dev", "/dev")
	args = append(args, "--perms", "1777", "--tmpfs", "/dev/shm")
	for _, path := range temporaryPaths {
		args = append(args, "--perms", "1777", "--tmpfs", path)
	}
	if config.PrivateWorkspace {
		parentIsTemporary := false
		for _, path := range temporaryPaths {
			if pathWithinAny(workspace, []string{path}) {
				parentIsTemporary = true
				break
			}
		}
		if parentIsTemporary {
			args = append(args, "--perms", "0700", "--dir", workspace)
		} else {
			args = append(args, "--perms", "0700", "--tmpfs", workspace)
		}
	}

	explicitReads := append([]string(nil), config.ExtraReadPaths...)
	if !config.PrivateWorkspace || workingDirectory != workspace {
		explicitReads = append(explicitReads, workingDirectory)
	}
	explicitReads = append(explicitReads, config.Command)
	resolverPaths := config.ResolverPaths
	if config.AllowNetwork && resolverPaths == nil {
		resolverPaths = bubblewrapResolverPaths()
	}
	if config.AllowNetwork {
		explicitReads = append(explicitReads, resolverPaths...)
	}
	explicitReads, err = cleanAbsolutePaths(explicitReads, "sandbox read path")
	if err != nil {
		return nil, err
	}
	for _, path := range explicitReads {
		if isFilesystemRoot(path) {
			return nil, errors.New("filesystem root cannot be an extra sandbox read path")
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve sandbox read path %q: %w", path, resolveErr)
		}
		resolved = filepath.Clean(resolved)
		if isFilesystemRoot(resolved) {
			return nil, fmt.Errorf("sandbox read path %q resolves to the filesystem root", path)
		}
		if _, statErr := os.Stat(resolved); statErr != nil {
			return nil, fmt.Errorf("stat sandbox read path %q: %w", resolved, statErr)
		}
		pathVisible := pathWithinAny(path, visibleReadRoots) || pathWithinAny(path, writeRoots)
		resolvedVisible := pathWithinAny(resolved, visibleReadRoots) || pathWithinAny(resolved, writeRoots)
		if pathVisible && resolvedVisible {
			continue
		}
		destination := path
		if pathVisible {
			// The visible parent already contains the symlink. Mount its target
			// where that symlink resolves instead of replacing the link itself.
			destination = resolved
		}
		args = append(args, "--ro-bind", resolved, destination)
		visibleReadRoots = append(visibleReadRoots, destination)
	}

	// Writable mounts come last so an explicitly-authorized project nested
	// beneath a read-only system or temp mount remains writable.
	for _, path := range writeRoots {
		args = append(args, "--bind", path, path)
	}
	// Bubblewrap's otherwise-empty root is a tmpfs. Lock it after creating mount
	// points; --remount-ro is non-recursive, so the project binds stay writable.
	args = append(args, "--remount-ro", "/", "--chdir", workingDirectory, "--", config.Command)
	args = append(args, config.Args...)
	return args, nil
}

func appendBubblewrapSystemPath(args []string, path string, systemRoots []string) ([]string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return args, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect Linux system path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return append(args, "--ro-bind", path, path), true, nil
	}
	target, err := os.Readlink(path)
	if err != nil {
		return nil, false, fmt.Errorf("read Linux system symlink %q: %w", path, err)
	}
	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(filepath.Dir(path), targetPath)
	}
	targetPath = filepath.Clean(targetPath)
	if pathWithinAny(targetPath, systemRoots) {
		return append(args, "--symlink", target, path), true, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, false, fmt.Errorf("resolve Linux system symlink %q: %w", path, err)
	}
	return append(args, "--ro-bind", filepath.Clean(resolved), path), true, nil
}

func bubblewrapResolverPaths() []string {
	const resolvConf = "/etc/resolv.conf"
	resolved, err := filepath.EvalSymlinks(resolvConf)
	if err != nil {
		return nil
	}
	resolved = filepath.Clean(resolved)
	if resolved == resolvConf || pathWithinAny(resolved, defaultBubblewrapSystemPaths) {
		return nil
	}
	return []string{resolved}
}

func cleanAbsolutePaths(paths []string, label string) ([]string, error) {
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if strings.IndexByte(path, 0) >= 0 {
			return nil, fmt.Errorf("%s contains NUL", label)
		}
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("%s must be absolute: %q", label, path)
		}
		unique[filepath.Clean(path)] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Slice(result, func(i, j int) bool {
		leftDepth := strings.Count(result[i], string(filepath.Separator))
		rightDepth := strings.Count(result[j], string(filepath.Separator))
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return result[i] < result[j]
	})
	return result, nil
}

func uniqueOrderedPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func pathWithinAny(path string, roots []string) bool {
	for _, root := range roots {
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func isFilesystemRoot(path string) bool {
	return filepath.Dir(path) == path
}
