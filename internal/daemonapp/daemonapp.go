// Package daemonapp runs the Dire Agent HTTP and WebSocket daemon.
package daemonapp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/internal/buildinfo"
	"github.com/imeredith/dire-agent/internal/lifecycle"
	"github.com/imeredith/dire-agent/internal/webui"
	"github.com/imeredith/dire-agent/provider/codex"
	"github.com/imeredith/dire-agent/threadstore"
)

// Run starts the daemon and blocks until it is stopped.
func Run(arguments []string) error {
	flags := flag.NewFlagSet("dire-agent daemon", flag.ContinueOnError)
	environmentControlToken := os.Getenv("DIRE_AGENT_CONTROL_TOKEN")
	_ = os.Unsetenv("DIRE_AGENT_CONTROL_TOKEN")
	address := flags.String("addr", "127.0.0.1:7331", "HTTP/WebSocket listen address")
	dataDirectory := flags.String("data-dir", "", "directory containing one SQLite file per project")
	configPath := flags.String("config", "", "versioned daemon configuration file")
	authFile := flags.String("auth-file", "", "Codex CLI auth.json path")
	defaultModel := flags.String("model", "gpt-5.6", "default model")
	defaultCWD := flags.String("cwd", "", "default project folder")
	defaultTools := flags.String("tools", "read,grep,find,ls", "comma-separated default tool allowlist")
	webDirectory := flags.String("web-dir", "", "serve the production Web UI from this Vite dist directory")
	disableWebUI := flags.Bool("no-web-ui", false, "disable the embedded production Web UI")
	allowRemote := flags.Bool("allow-remote", false, "allow listening on a non-loopback address without transport authentication")
	projectProxy := flags.Bool("project-proxy", true, "proxy loopback project servers under /project/server/{port}")
	allowRemoteProjectProxy := flags.Bool("allow-remote-project-proxy", false, "allow the project-server proxy on a non-loopback listener")
	runtimeFile := flags.String("runtime-file", "", "managed-daemon runtime state file")
	instanceID := flags.String("instance-id", "", "managed-daemon instance identifier")
	controlToken := flags.String("control-token", environmentControlToken, "managed-daemon control token")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if err := ValidateListenerSecurity(*address, *allowRemote, *projectProxy, *allowRemoteProjectProxy); err != nil {
		return err
	}
	if *disableWebUI && strings.TrimSpace(*webDirectory) != "" {
		return errors.New("-no-web-ui and -web-dir cannot be used together")
	}
	managed := strings.TrimSpace(*runtimeFile) != ""
	if managed && (strings.TrimSpace(*instanceID) == "" || strings.TrimSpace(*controlToken) == "") {
		return errors.New("managed daemon requires -instance-id and -control-token")
	}
	if managed && !filepath.IsAbs(*runtimeFile) {
		return errors.New("managed daemon runtime file must be an absolute path")
	}
	if managed && (len(*instanceID) < 16 || len(*controlToken) < 32) {
		return errors.New("managed daemon instance id or control token is too short")
	}
	if !managed {
		var err error
		*instanceID, err = lifecycle.NewInstanceID()
		if err != nil {
			return fmt.Errorf("generate daemon instance id: %w", err)
		}
	}

	var webFiles fs.FS
	var webSource string
	if !*disableWebUI {
		var err error
		webFiles, webSource, err = LoadWebUI(*webDirectory)
		if err != nil {
			return err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if *dataDirectory == "" {
		*dataDirectory = DefaultDataDirectory(home)
	}
	if *configPath == "" {
		*configPath = configuration.DefaultPath(home)
	}
	configStore, err := configuration.New(*configPath)
	if err != nil {
		return err
	}
	loadedConfig, err := configStore.Load(context.Background())
	if err != nil {
		return err
	}
	if *defaultModel == "gpt-5.6" && loadedConfig.Global.Model.ID != "" {
		*defaultModel = loadedConfig.Global.Model.ID
	}
	if *defaultTools == "read,grep,find,ls" && len(loadedConfig.Global.Tools.Enabled) > 0 {
		*defaultTools = strings.Join(loadedConfig.Global.Tools.Enabled, ",")
	}
	if *defaultCWD == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		*defaultCWD = cwd
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, stopDaemon := context.WithCancel(ctx)
	defer stopDaemon()
	provider, err := codex.New(ctx, codex.Config{AuthFile: *authFile, DefaultModel: *defaultModel})
	if err != nil {
		return err
	}
	store, err := threadstore.New(*dataDirectory)
	if err != nil {
		provider.Close()
		return err
	}
	mcpSource := capability.NewMCPSource(capability.MCPSourceConfig{})
	extensionSource := capability.NewExtensionSource(capability.ExtensionSourceOptions{})
	capabilities := capability.NewRegistry(capability.RegistryConfig{
		Settings: configStore,
		Defaults: loadedConfig.Global,
		Sources:  []capability.Source{mcpSource, extensionSource},
	})
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultModel: *defaultModel, DefaultCWD: *defaultCWD,
		DefaultTools: SplitList(*defaultTools), DefaultThinking: string(loadedConfig.Global.Thinking.Level),
		Settings: configStore, Capabilities: capabilities,
	})
	if err != nil {
		provider.Close()
		return err
	}
	defer manager.Close()

	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *address, err)
	}
	defer listener.Close()
	boundAddress := listener.Addr().String()
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve daemon executable: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	health := lifecycle.Health{
		Service:    lifecycle.ServiceName,
		Status:     "ok",
		Version:    buildinfo.Version,
		PID:        os.Getpid(),
		InstanceID: *instanceID,
	}
	server := &http.Server{
		Handler: (&daemon.Server{
			Manager:                 manager,
			Config:                  configStore,
			WebUI:                   webFiles,
			ProjectProxyEnabled:     *projectProxy,
			ProjectProxyBlockedPort: AddressPort(boundAddress),
			Health:                  health,
			ControlToken:            *controlToken,
			Shutdown:                stopDaemon,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	if managed {
		state := lifecycle.RuntimeState{
			Schema:       lifecycle.RuntimeSchema,
			PID:          os.Getpid(),
			InstanceID:   *instanceID,
			ControlToken: *controlToken,
			Version:      buildinfo.Version,
			Executable:   executable,
			HTTPURL:      "http://" + boundAddress,
			StartedAt:    time.Now().UTC(),
		}
		if err := lifecycle.WriteRuntime(*runtimeFile, state); err != nil {
			return fmt.Errorf("write managed daemon state: %w", err)
		}
		defer lifecycle.RemoveRuntimeIfInstance(*runtimeFile, *instanceID)
	}

	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	webStatus := "disabled"
	if webSource != "" {
		webStatus = webSource
	}
	proxyStatus := "disabled"
	if *projectProxy {
		proxyStatus = "/project/server/{port}"
	}
	fmt.Fprintf(os.Stderr, "dire-agent daemon listening on http://%s (projects: %s, config: %s, web: %s, project proxy: %s)\n", boundAddress, store.Directory(), configStore.Path(), webStatus, proxyStatus)

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func LoadWebUI(directory string) (fs.FS, string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		files, ok := webui.Files()
		if !ok {
			return nil, "", nil
		}
		return files, "embedded", nil
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, "", fmt.Errorf("resolve Web UI directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, "", fmt.Errorf("resolve Web UI directory %q: %w", directory, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, "", fmt.Errorf("inspect Web UI directory %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("Web UI path is not a directory: %s", resolved)
	}
	indexInfo, err := os.Stat(filepath.Join(resolved, "index.html"))
	if err != nil {
		return nil, "", fmt.Errorf("Web UI directory has no index.html: %w", err)
	}
	if !indexInfo.Mode().IsRegular() {
		return nil, "", errors.New("Web UI index.html is not a regular file")
	}
	return os.DirFS(resolved), resolved, nil
}

func DefaultDataDirectory(home string) string {
	projects := filepath.Join(home, ".dire-agent", "projects")
	for _, candidate := range []string{
		projects,
		filepath.Join(home, ".dire-agent", "threads"),
		filepath.Join(home, ".goagent", "projects"),
		filepath.Join(home, ".goagent", "threads"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return projects
}

func SplitList(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func IsLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ValidateListenerSecurity(address string, allowRemote, projectProxy, allowRemoteProjectProxy bool) error {
	loopback := IsLoopbackAddress(address)
	if !loopback && !allowRemote {
		return errors.New("refusing non-loopback listen address without -allow-remote")
	}
	if !loopback && projectProxy && !allowRemoteProjectProxy {
		return errors.New("refusing to expose the project-server proxy on a non-loopback listener without -allow-remote-project-proxy (or pass -project-proxy=false)")
	}
	return nil
}

func AddressPort(address string) int {
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}
