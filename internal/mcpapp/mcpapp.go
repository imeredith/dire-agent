// Package mcpapp implements the Dire Agent MCP server command.
package mcpapp

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/imeredith/dire-agent/client"
	"github.com/imeredith/dire-agent/mcpserver"
)

// Run executes the MCP server with the supplied command-line arguments.
func Run(arguments []string) error {
	flags := flag.NewFlagSet("dire-agent mcp", flag.ContinueOnError)
	daemonURL := flags.String("daemon", "ws://127.0.0.1:7331/ws", "Dire Agent daemon WebSocket URL")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backend, err := client.Dial(ctx, *daemonURL)
	if err != nil {
		return err
	}
	defer backend.Close()
	server, err := mcpserver.New(backend)
	if err != nil {
		return err
	}
	return server.RunStdio(ctx)
}
