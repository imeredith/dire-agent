package capability

import (
	"os"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/extensions"
	localtools "github.com/dire-kiwi/dire-agent/tools"
)

func sandboxExtensionSources(input []extensions.Source, scope Scope, mode configuration.SandboxMode) ([]extensions.Source, []Descriptor) {
	if mode == configuration.SandboxOff {
		return input, nil
	}
	result := append([]extensions.Source(nil), input...)
	var diagnostics []Descriptor
	for index := range result {
		source := &result[index]
		if !source.Enabled || source.Trust != extensions.TrustTrusted || source.Command == "" {
			continue
		}
		workspace := scope.CWD
		if workspace == "" {
			workspace = os.TempDir()
		}
		command, args, err := localtools.WrapSandboxedProcess(localtools.ProcessSandbox{
			Workspace: workspace, Command: source.Command, Args: source.Args,
			ExtraReadPaths:       []string{source.Location},
			AdditionalWritePaths: scope.AdditionalFolders,
			AllowNetwork:         mode == configuration.SandboxWorkspace,
		})
		if err != nil {
			diagnostics = append(diagnostics, Descriptor{
				Name: "extension:" + extensionID(source.ID) + ":sandbox", Source: "extension",
				Description: err.Error(), Enabled: false, Status: "sandbox_unavailable",
			})
			source.Command = ""
			source.Args = nil
			continue
		}
		source.Command, source.Args = command, args
	}
	return result, diagnostics
}
