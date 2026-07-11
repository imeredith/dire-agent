package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/extensions"
)

type remoteExtensionSetting struct {
	name   string
	source configuration.ExtensionSource
}

func configuredExtensionSources(settings configuration.ExtensionSettings) ([]extensions.Source, []remoteExtensionSetting) {
	names := make([]string, 0, len(settings.Sources))
	for name := range settings.Sources {
		names = append(names, name)
	}
	sort.Strings(names)
	var local []extensions.Source
	var remote []remoteExtensionSetting
	for _, name := range names {
		source := settings.Sources[name]
		if source.Kind != configuration.ExtensionLocal {
			remote = append(remote, remoteExtensionSetting{name: name, source: source})
			continue
		}
		local = append(local, extensions.Source{
			ID: name, Location: source.Location, Enabled: source.Enabled,
			Trust: extensionTrust(source.Trust), Command: source.Command,
			Args: append([]string(nil), source.Args...), Env: cloneExtensionEnv(source.Env),
			InheritEnv: source.InheritEnv,
		})
	}
	return local, remote
}

func extensionTrust(trust configuration.TrustMode) extensions.Trust {
	switch trust {
	case configuration.TrustDenied:
		return extensions.TrustDenied
	case configuration.TrustTrusted:
		return extensions.TrustTrusted
	default:
		return extensions.TrustPrompt
	}
}

func remoteExtensionDescriptor(name string, source configuration.ExtensionSource) Descriptor {
	status := "install_unsupported"
	if !source.Enabled {
		status = "disabled"
	} else if source.Trust == configuration.TrustDenied {
		status = "denied"
	} else if source.Trust != configuration.TrustTrusted {
		status = "needs_trust"
	}
	return Descriptor{
		Name: "extension:" + extensionID(name), Source: "extension",
		Description: "Remote extension is catalogued; automatic installation is not supported.",
		Enabled:     false, Status: status,
	}
}

func extensionDescriptor(extension extensions.Extension) Descriptor {
	description := strings.Join(strings.Fields(extension.Description), " ")
	if description == "" {
		description = string(extension.Format) + " extension"
	}
	enabled := extension.State == extensions.StateRunnable || extension.State == extensions.StateCatalogued
	return Descriptor{
		Name: "extension:" + extension.ID, Source: "extension", Description: description,
		Enabled: enabled, Status: string(extension.State),
	}
}

func descriptorForExtensionTool(tool agentloop.Tool, extensionID string) Descriptor {
	return descriptor(tool, "extension:"+extensionID, true, "ready")
}

func safeExtensionDiagnostic(id, code, severity string, index int) Descriptor {
	if id == "" {
		id = "catalog"
	}
	if code == "" {
		code = "unknown"
	}
	return Descriptor{
		Name:   "extension:" + extensionID(id) + ":diagnostic:" + extensionID(code) + ":" + itoa(index),
		Source: "extension", Description: "Extension diagnostic: " + code + ".",
		Enabled: false, Status: severity,
	}
}

func extensionFingerprint(extension extensions.Extension) string {
	data, _ := json.Marshal(struct {
		ID      string
		Version string
		Process extensions.ProcessSpec
	}{extension.ID, extension.Version, extension.Process})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func extensionID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var result strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			result.WriteRune(char)
		} else if result.Len() > 0 {
			result.WriteByte('_')
		}
	}
	id := strings.Trim(result.String(), "_-")
	if id == "" {
		return "extension"
	}
	return id
}

func cloneExtensionEnv(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func itoa(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var output [20]byte
	position := len(output)
	for value > 0 {
		position--
		output[position] = digits[value%10]
		value /= 10
	}
	return string(output[position:])
}
