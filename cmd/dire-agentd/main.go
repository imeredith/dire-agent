package main

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
	"github.com/imeredith/dire-agent/internal/webui"
	"github.com/imeredith/dire-agent/provider/codex"
	"github.com/imeredith/dire-agent/threadstore"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dire-agentd:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("dire-agentd", flag.ContinueOnError)
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
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if err := validateListenerSecurity(*address, *allowRemote, *projectProxy, *allowRemoteProjectProxy); err != nil {
		return err
	}
	if *disableWebUI && strings.TrimSpace(*webDirectory) != "" {
		return errors.New("-no-web-ui and -web-dir cannot be used together")
	}
	var webFiles fs.FS
	var webSource string
	if !*disableWebUI {
		var err error
		webFiles, webSource, err = loadWebUI(*webDirectory)
		if err != nil {
			return err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if *dataDirectory == "" {
		*dataDirectory = defaultDataDirectory(home)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
		DefaultTools: splitList(*defaultTools), DefaultThinking: string(loadedConfig.Global.Thinking.Level),
		Settings: configStore, Capabilities: capabilities,
	})
	if err != nil {
		provider.Close()
		return err
	}
	defer manager.Close()

	server := &http.Server{
		Addr: *address,
		Handler: (&daemon.Server{
			Manager:                 manager,
			Config:                  configStore,
			WebUI:                   webFiles,
			ProjectProxyEnabled:     *projectProxy,
			ProjectProxyBlockedPort: addressPort(*address),
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serverErrors := make(chan error, 1)
	go func() {
		webStatus := "disabled"
		if webSource != "" {
			webStatus = webSource
		}
		proxyStatus := "disabled"
		if *projectProxy {
			proxyStatus = "/project/server/{port}"
		}
		fmt.Fprintf(os.Stderr, "dire-agentd listening on http://%s (projects: %s, config: %s, web: %s, project proxy: %s)\n", *address, store.Directory(), configStore.Path(), webStatus, proxyStatus)
		serverErrors <- server.ListenAndServe()
	}()

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

func loadWebUI(directory string) (fs.FS, string, error) {
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

func defaultDataDirectory(home string) string {
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

func splitList(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isLoopbackAddress(address string) bool {
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

func validateListenerSecurity(address string, allowRemote, projectProxy, allowRemoteProjectProxy bool) error {
	loopback := isLoopbackAddress(address)
	if !loopback && !allowRemote {
		return errors.New("refusing non-loopback listen address without -allow-remote")
	}
	if !loopback && projectProxy && !allowRemoteProjectProxy {
		return errors.New("refusing to expose the project-server proxy on a non-loopback listener without -allow-remote-project-proxy (or pass -project-proxy=false)")
	}
	return nil
}

func addressPort(address string) int {
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
