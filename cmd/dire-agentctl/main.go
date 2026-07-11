package main

import (
	"fmt"
	"os"

	"github.com/imeredith/dire-agent/internal/controlapp"
)

func main() {
	if err := controlapp.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dire-agentctl:", err)
		os.Exit(1)
	}
}
