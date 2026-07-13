package main

import (
	"fmt"
	"os"

	"github.com/dire-kiwi/dire-agent/internal/mcpapp"
)

func main() {
	if err := mcpapp.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dire-agent-mcp:", err)
		os.Exit(1)
	}
}
