package capability

import (
	"os"
	"path/filepath"

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
		privateWorkspace := false
		if workspace == "" {
			workspace = os.TempDir()
			privateWorkspace = true
		}
		workingDirectory := extensionSandboxWorkingDirectory(source.Location)
		command, args, err := localtools.WrapSandboxedProcess(localtools.ProcessSandbox{
			Workspace: workspace, WorkingDirectory: workingDirectory,
			Command: source.Command, Args: source.Args,
			ExtraReadPaths:       []string{source.Location},
			AdditionalWritePaths: scope.AdditionalFolders,
			AllowNetwork:         mode == configuration.SandboxWorkspace,
			PrivateWorkspace:     privateWorkspace,
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
		source.Sandboxed = true
	}
	return result, diagnostics
}

func extensionSandboxWorkingDirectory(location string) string {
	info, err := os.Stat(location)
	if err != nil || info.IsDir() {
		return location
	}
	directory := filepath.Dir(location)
	if filepath.Base(location) == "plugin.json" && filepath.Base(directory) == ".codex-plugin" {
		return filepath.Dir(directory)
	}
	return directory
}
