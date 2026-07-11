package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/imeredith/dire-agent/internal/daemonapp"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dire-agentd:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error { return daemonapp.Run(arguments) }

// Compatibility aliases keep the legacy command's tests and downstream users
// working while the implementation lives in the shared daemon application.
func loadWebUI(directory string) (fs.FS, string, error) { return daemonapp.LoadWebUI(directory) }
func defaultDataDirectory(home string) string           { return daemonapp.DefaultDataDirectory(home) }
func addressPort(address string) int                    { return daemonapp.AddressPort(address) }
func validateListenerSecurity(address string, allowRemote, projectProxy, allowRemoteProjectProxy bool) error {
	return daemonapp.ValidateListenerSecurity(address, allowRemote, projectProxy, allowRemoteProjectProxy)
}
