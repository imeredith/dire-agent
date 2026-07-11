//go:build windows

package daemon

import "os/exec"

// CommandContext terminates the PowerShell parent on cancellation. Windows job
// object support can strengthen descendant cleanup when the daemon supports a
// native Windows sandbox.
func configureSetupCommand(_ *exec.Cmd) {}
