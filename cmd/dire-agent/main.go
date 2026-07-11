package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/internal/buildinfo"
	"github.com/imeredith/dire-agent/internal/controlapp"
	"github.com/imeredith/dire-agent/internal/daemonapp"
	"github.com/imeredith/dire-agent/internal/lifecycle"
	"github.com/imeredith/dire-agent/internal/mcpapp"
	"github.com/imeredith/dire-agent/internal/updater"
	"github.com/imeredith/dire-agent/provider/codex"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "dire-agent:", err)
		os.Exit(1)
	}
}

func run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(arguments) > 0 {
		switch arguments[0] {
		case "daemon":
			return daemonapp.Run(arguments[1:])
		case "mcp":
			return mcpapp.Run(arguments[1:])
		case "ask":
			return runAsk(arguments[1:], stdin, stdout, stderr)
		case "version", "--version", "-version":
			if len(arguments) != 1 {
				return fmt.Errorf("%s does not accept arguments", arguments[0])
			}
			_, err := fmt.Fprintf(stdout, "dire-agent %s\n", buildinfo.String())
			return err
		case "help", "--help", "-h":
			printUsage(stdout)
			return nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	executable, err := canonicalExecutable()
	if err != nil {
		return err
	}
	supervisor, err := lifecycle.NewSupervisor(home, executable)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(arguments) > 0 {
		switch arguments[0] {
		case "start":
			if len(arguments) != 1 {
				return errors.New("start does not accept arguments")
			}
			operation, err := supervisor.AcquireOperationLock(ctx)
			if err != nil {
				return err
			}
			defer operation.Close()
			started, err := supervisor.Start(ctx)
			if err != nil {
				return err
			}
			if started {
				fmt.Fprintln(stdout, "Dire Agent daemon started")
			} else {
				fmt.Fprintln(stdout, "Dire Agent daemon is already running")
			}
			return nil
		case "stop":
			if len(arguments) != 1 {
				return errors.New("stop does not accept arguments")
			}
			operation, err := supervisor.AcquireOperationLock(ctx)
			if err != nil {
				return err
			}
			defer operation.Close()
			stopped, err := supervisor.Stop(ctx)
			if err != nil {
				return err
			}
			if stopped {
				fmt.Fprintln(stdout, "Dire Agent daemon stopped")
			} else {
				fmt.Fprintln(stdout, "Dire Agent daemon is not running")
			}
			return nil
		case "status":
			if len(arguments) != 1 {
				return errors.New("status does not accept arguments")
			}
			status, err := supervisor.Status(ctx)
			if err != nil {
				return err
			}
			if !status.Running {
				if status.Managed && status.PID > 1 && !status.Stale {
					fmt.Fprintf(stdout, "Dire Agent daemon is not healthy (pid %d: %s; log: %s)\n", status.PID, status.Detail, status.LogFile)
					return nil
				}
				fmt.Fprintln(stdout, "Dire Agent daemon is stopped")
				return nil
			}
			management := "managed"
			if !status.Managed {
				management = "unmanaged"
			}
			fmt.Fprintf(stdout, "Dire Agent daemon is running (%s, pid %d, version %s, %s)\n", management, status.PID, status.Version, status.HTTPURL)
			return nil
		case "upgrade":
			return runUpgrade(ctx, arguments[1:], supervisor, executable, stdout, stderr)
		case "tui":
			arguments = arguments[1:]
			if len(arguments) > 0 && (arguments[0] == "-h" || arguments[0] == "--help") {
				err := controlapp.Run(arguments)
				if errors.Is(err, flag.ErrHelp) {
					return nil
				}
				return err
			}
		}
	}

	operation, err := supervisor.AcquireOperationLock(ctx)
	if err != nil {
		return fmt.Errorf("lock daemon startup: %w", err)
	}
	started, err := supervisor.Start(ctx)
	closeErr := operation.Close()
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("unlock daemon startup: %w", closeErr)
	}
	if started {
		fmt.Fprintln(stderr, "Dire Agent daemon started")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find current directory: %w", err)
	}
	controlArguments := append([]string{"-folder", cwd}, arguments...)
	return controlapp.Run(controlArguments)
}

func runUpgrade(ctx context.Context, arguments []string, supervisor *lifecycle.Supervisor, executable string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("dire-agent upgrade", flag.ContinueOnError)
	flags.SetOutput(stderr)
	version := flags.String("version", "", "release version to install (default: latest)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("upgrade does not accept positional arguments")
	}

	status, err := supervisor.Status(ctx)
	if err != nil {
		return fmt.Errorf("inspect daemon before upgrade: %w", err)
	}
	if err := validateUpgradeStatus(status); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Downloading and verifying Dire Agent upgrade...")
	prepared, err := updater.New(executable).Prepare(ctx, *version)
	if err != nil {
		return err
	}
	defer prepared.Cleanup()
	operation, err := supervisor.AcquireOperationLock(ctx)
	if err != nil {
		return fmt.Errorf("lock upgrade: %w", err)
	}
	defer operation.Close()
	status, err = supervisor.Status(ctx)
	if err != nil {
		return fmt.Errorf("recheck daemon before upgrade: %w", err)
	}
	if err := validateUpgradeStatus(status); err != nil {
		return err
	}
	if sameVersion(buildinfo.Version, prepared.Version) {
		if status.Running && status.Managed && !sameVersion(status.Version, prepared.Version) {
			if _, err := supervisor.Stop(ctx); err != nil {
				return fmt.Errorf("stop outdated daemon: %w", err)
			}
			if _, err := supervisor.Start(ctx); err != nil {
				return fmt.Errorf("Dire Agent %s is installed, but its daemon did not restart: %w", prepared.Version, err)
			}
			fmt.Fprintf(stdout, "Dire Agent %s is installed; restarted its daemon\n", prepared.Version)
			return nil
		}
		fmt.Fprintf(stdout, "Dire Agent %s is already installed\n", prepared.Version)
		return nil
	}

	restart := status.Running && status.Managed
	if restart {
		if _, err := supervisor.Stop(ctx); err != nil {
			return fmt.Errorf("stop daemon for upgrade: %w", err)
		}
	}
	if err := prepared.Apply(); err != nil {
		if restart {
			_, _ = supervisor.Start(context.Background())
		}
		return err
	}
	if restart {
		if _, err := supervisor.Start(ctx); err != nil {
			return fmt.Errorf("Dire Agent %s was installed, but its daemon did not restart: %w", prepared.Version, err)
		}
	}
	fmt.Fprintf(stdout, "Upgraded Dire Agent from %s to %s\n", buildinfo.Version, prepared.Version)
	return nil
}

func validateUpgradeStatus(status lifecycle.Status) error {
	if status.Running && !status.Managed {
		return errors.New("a foreground or unmanaged daemon is running; stop it before upgrading")
	}
	if status.Managed && !status.Running && !status.Stale && status.PID > 1 {
		return fmt.Errorf("managed daemon pid %d is not healthy; refusing to upgrade it (%s)", status.PID, status.Detail)
	}
	return nil
}

func sameVersion(left, right string) bool {
	normalize := func(value string) string { return strings.TrimPrefix(strings.TrimSpace(value), "v") }
	return normalize(left) != "" && normalize(left) == normalize(right)
}

func canonicalExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find current executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	return executable, nil
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, `Dire Agent

Usage:
  dire-agent [message]       Start the daemon if needed and open the TUI
  dire-agent tui [options]   Open the TUI
  dire-agent start           Start the background daemon
  dire-agent stop            Stop the managed daemon
  dire-agent status          Show daemon status
  dire-agent upgrade         Install the latest release and restart if needed
  dire-agent version         Show version information
  dire-agent ask [options]   Run the legacy one-shot agent

Advanced:
  dire-agent mcp [options]   Run the MCP bridge over stdio
  dire-agent daemon          Run the daemon in the foreground`)
}

func runAsk(arguments []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("dire-agent ask", flag.ContinueOnError)
	flags.SetOutput(stderr)
	model := flags.String("model", "gpt-5.6", "Codex model")
	instructions := flags.String("instructions", "", "developer instructions for the agent")
	authFile := flags.String("auth-file", "", "Codex CLI auth.json path")
	timeout := flags.Duration("timeout", 10*time.Minute, "maximum time for the request")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *timeout <= 0 {
		return errors.New("timeout must be greater than zero")
	}

	prompt, err := readPrompt(flags.Args(), stdin)
	if err != nil {
		return err
	}
	baseContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(baseContext, *timeout)
	defer cancel()

	provider, err := codex.New(ctx, codex.Config{AuthFile: *authFile})
	if err != nil {
		return err
	}
	defer provider.Close()

	aiAgent, err := agent.New(ctx, provider, agent.SessionOptions{
		Model:        *model,
		Instructions: *instructions,
	})
	if err != nil {
		return err
	}
	result, err := aiAgent.Run(ctx, prompt)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, result.Text)
	return err
}

func readPrompt(arguments []string, stdin io.Reader) (string, error) {
	if len(arguments) != 0 {
		prompt := strings.TrimSpace(strings.Join(arguments, " "))
		if prompt == "" {
			return "", errors.New("prompt is empty")
		}
		return prompt, nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", errors.New("provide a prompt as arguments or on stdin")
	}
	return prompt, nil
}
