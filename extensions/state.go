package extensions

import (
	"fmt"
	"path/filepath"
	"strings"
)

func baseEntry(source Source, id, name, root string) Extension {
	trust := source.Trust
	if trust == "" {
		trust = TrustPrompt
	}
	return Extension{
		ID: id, Name: name, Root: root, Enabled: source.Enabled, Trust: trust,
		Process: ProcessSpec{
			Command: strings.TrimSpace(source.Command), Args: append([]string(nil), source.Args...),
			Dir: root, Env: cloneStrings(source.Env), InheritEnv: source.InheritEnv, Sandboxed: source.Sandboxed,
		},
	}
}

func finishState(entry *Extension) {
	if entry.ID == "" {
		entry.Diagnostics = append(entry.Diagnostics, Diagnostic{Severity: SeverityError, Code: "id-invalid", Message: "extension id is empty", Path: entry.Root, ExtensionID: entry.ID})
	}
	if entry.Trust != TrustDenied && entry.Trust != TrustPrompt && entry.Trust != TrustTrusted {
		entry.Diagnostics = append(entry.Diagnostics, Diagnostic{Severity: SeverityError, Code: "trust-invalid", Message: fmt.Sprintf("invalid trust state %q", entry.Trust), Path: entry.Root, ExtensionID: entry.ID})
	}
	if err := validateProcessSpec(entry.Process, entry.Process.Command == ""); err != nil {
		entry.Diagnostics = append(entry.Diagnostics, Diagnostic{Severity: SeverityError, Code: "process-invalid", Message: err.Error(), Path: entry.Root, ExtensionID: entry.ID})
	}
	for _, diagnostic := range entry.Diagnostics {
		if diagnostic.Severity == SeverityError {
			entry.State = StateInvalid
			return
		}
	}
	if !entry.Enabled {
		entry.State = StateDisabled
		return
	}
	switch entry.Trust {
	case TrustDenied:
		entry.State = StateDenied
	case TrustPrompt:
		entry.State = StateNeedsTrust
	case TrustTrusted:
		if entry.Process.Command == "" {
			entry.State = StateCatalogued
		} else {
			entry.State = StateRunnable
		}
	}
}

func validateProcessSpec(spec ProcessSpec, allowEmpty bool) error {
	if spec.Command == "" && !allowEmpty {
		return fmt.Errorf("command is required")
	}
	if strings.ContainsRune(spec.Command, 0) {
		return fmt.Errorf("command contains NUL")
	}
	if spec.Dir != "" && !filepath.IsAbs(spec.Dir) {
		return fmt.Errorf("working directory must be absolute")
	}
	for _, argument := range spec.Args {
		if strings.ContainsRune(argument, 0) {
			return fmt.Errorf("argument contains NUL")
		}
	}
	for key, value := range spec.Env {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("invalid environment entry %q", key)
		}
	}
	return nil
}

func cloneStrings(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
