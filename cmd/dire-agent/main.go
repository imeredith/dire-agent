package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/provider/codex"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "dire-agent:", err)
		os.Exit(1)
	}
}

func run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("dire-agent", flag.ContinueOnError)
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
