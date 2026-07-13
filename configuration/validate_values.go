package configuration

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

func validateSubagents(settings SubagentSettings, scope string) error {
	if settings.MaxDepth < 1 || settings.MaxDepth > 4 {
		return fmt.Errorf("configuration: %s subagent max depth must be between 1 and 4", scope)
	}
	if settings.MaxChildren < 1 || settings.MaxChildren > 32 {
		return fmt.Errorf("configuration: %s subagent max children must be between 1 and 32", scope)
	}
	if settings.MaxConcurrent < 1 || settings.MaxConcurrent > 16 {
		return fmt.Errorf("configuration: %s subagent max concurrent must be between 1 and 16", scope)
	}
	if len(settings.Profiles) == 0 {
		return fmt.Errorf("configuration: %s requires at least one subagent profile", scope)
	}
	for name, profile := range settings.Profiles {
		if !configName.MatchString(name) {
			return fmt.Errorf("configuration: %s invalid subagent profile %q", scope, name)
		}
		if strings.TrimSpace(profile.Description) == "" {
			return fmt.Errorf("configuration: %s subagent profile %q requires a description", scope, name)
		}
		if profile.Thinking != "" && !validThinking(profile.Thinking) {
			return fmt.Errorf("configuration: %s subagent profile %q has invalid thinking", scope, name)
		}
		if err := uniqueNonEmpty(profile.Tools, scope+" subagent profile "+name+" tools"); err != nil {
			return err
		}
	}
	return nil
}

func validateExtensions(settings ExtensionSettings, scope string) error {
	for name, source := range settings.Sources {
		if !configName.MatchString(name) {
			return fmt.Errorf("configuration: %s invalid extension name %q", scope, name)
		}
		if strings.TrimSpace(source.Location) == "" {
			return fmt.Errorf("configuration: extension %q location is required", name)
		}
		if !validTrust(source.Trust) {
			return fmt.Errorf("configuration: extension %q has invalid trust", name)
		}
		if err := validateProcessValues(source.Command, source.Args, source.Env, "extension "+name); err != nil {
			return err
		}
		if err := validateExtensionSecretKeys(source, name); err != nil {
			return err
		}
		switch source.Kind {
		case ExtensionLocal:
			if !filepath.IsAbs(source.Location) {
				return fmt.Errorf("configuration: local extension %q location must be absolute", name)
			}
		case ExtensionGit, ExtensionRegistry:
			if source.Command != "" || len(source.Args) > 0 || len(source.Env) > 0 ||
				len(source.SecretEnv) > 0 || source.InheritEnv {
				return fmt.Errorf("configuration: remote extension %q cannot set local process options", name)
			}
		default:
			return fmt.Errorf("configuration: extension %q has invalid source kind %q", name, source.Kind)
		}
	}
	return nil
}

func validateExtensionSecretKeys(source ExtensionSource, name string) error {
	if err := uniqueNonEmpty(source.SecretEnv, "extension secret environment keys"); err != nil {
		return err
	}
	for _, key := range source.SecretEnv {
		if _, ok := source.Env[key]; !ok {
			return fmt.Errorf("configuration: extension %q secret environment %q is not configured", name, key)
		}
	}
	return nil
}

func validateProcessValues(command string, args []string, env map[string]string, label string) error {
	if strings.ContainsRune(command, 0) {
		return fmt.Errorf("configuration: %s command contains NUL", label)
	}
	for _, argument := range args {
		if strings.ContainsRune(argument, 0) {
			return fmt.Errorf("configuration: %s argument contains NUL", label)
		}
	}
	for key, value := range env {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("configuration: %s has invalid environment entry %q", label, key)
		}
	}
	return nil
}

func validateDesktop(settings DesktopSettings, scope string) error {
	if settings.CodexHome == "" || !filepath.IsAbs(settings.CodexHome) {
		return fmt.Errorf("configuration: %s Codex home must be absolute", scope)
	}
	if settings.DesktopConfig != "" && !filepath.IsAbs(settings.DesktopConfig) {
		return fmt.Errorf("configuration: %s desktop config path must be absolute", scope)
	}
	if settings.SyncMode != SyncOff && settings.SyncMode != SyncImport &&
		settings.SyncMode != SyncExport && settings.SyncMode != SyncBidirectional {
		return fmt.Errorf("configuration: %s invalid desktop sync mode %q", scope, settings.SyncMode)
	}
	return nil
}

func validateLaunchers(configured []ProjectLauncher, scope string) error {
	launchers := ResolveProjectLaunchers(configured)
	if len(launchers) > 64 {
		return fmt.Errorf("configuration: %s cannot configure more than 64 launchers", scope)
	}
	ids := make(map[string]struct{}, len(launchers))
	shortcuts := make(map[string]string, len(launchers))
	for _, launcher := range launchers {
		if !configName.MatchString(launcher.ID) {
			return fmt.Errorf("configuration: %s invalid launcher id %q", scope, launcher.ID)
		}
		if _, exists := ids[launcher.ID]; exists {
			return fmt.Errorf("configuration: %s contains duplicate launcher id %q", scope, launcher.ID)
		}
		ids[launcher.ID] = struct{}{}
		switch launcher.Icon {
		case "", "tool", "run", "debug", "test":
		default:
			return fmt.Errorf("configuration: %s launcher %q has an invalid icon", scope, launcher.ID)
		}
		if strings.TrimSpace(launcher.Label) == "" || len(launcher.Label) > 128 || containsControl(launcher.Label) {
			return fmt.Errorf("configuration: %s launcher %q has an invalid label", scope, launcher.ID)
		}
		if len(launcher.Command) > 4096 || len(launcher.Args) > 128 {
			return fmt.Errorf("configuration: %s launcher %q command is too large", scope, launcher.ID)
		}
		if err := validateProcessValues(launcher.Command, launcher.Args, nil, "launcher "+launcher.ID); err != nil {
			return err
		}
		for _, argument := range launcher.Args {
			if len(argument) > 4096 {
				return fmt.Errorf("configuration: %s launcher %q argument is too large", scope, launcher.ID)
			}
		}
		switch launcher.Kind {
		case LauncherTerminal:
			if strings.TrimSpace(launcher.Command) == "" && len(launcher.Args) != 0 {
				return fmt.Errorf("configuration: %s launcher %q login shell cannot set arguments", scope, launcher.ID)
			}
		case LauncherDesktop:
			if strings.TrimSpace(launcher.Command) == "" {
				return fmt.Errorf("configuration: %s desktop launcher %q command is required", scope, launcher.ID)
			}
		default:
			return fmt.Errorf("configuration: %s launcher %q has invalid kind %q", scope, launcher.ID, launcher.Kind)
		}
		shortcut := strings.ToLower(strings.TrimSpace(launcher.Shortcut))
		if shortcut == "" {
			continue
		}
		if len(shortcut) > 128 || containsControl(shortcut) {
			return fmt.Errorf("configuration: %s launcher %q has an invalid shortcut", scope, launcher.ID)
		}
		shortcut, err := normalizeLauncherShortcut(shortcut)
		if err != nil {
			return fmt.Errorf("configuration: %s launcher %q has an invalid shortcut", scope, launcher.ID)
		}
		if other, exists := shortcuts[shortcut]; exists {
			return fmt.Errorf("configuration: %s launchers %q and %q use the same shortcut", scope, other, launcher.ID)
		}
		shortcuts[shortcut] = launcher.ID
	}
	return nil
}

func normalizeLauncherShortcut(value string) (string, error) {
	aliases := map[string]string{
		"cmd": "meta", "command": "meta", "ctrl": "control", "option": "alt",
		"grave": "backquote", "`": "backquote",
	}
	modifiers := map[string]bool{"mod": false, "meta": false, "control": false, "shift": false, "alt": false}
	key := ""
	for _, raw := range strings.Split(strings.ToLower(strings.TrimSpace(value)), "+") {
		part := strings.TrimSpace(raw)
		if alias := aliases[part]; alias != "" {
			part = alias
		}
		if _, isModifier := modifiers[part]; isModifier {
			if modifiers[part] {
				return "", errors.New("duplicate modifier")
			}
			modifiers[part] = true
			continue
		}
		if part == "" || key != "" {
			return "", errors.New("shortcut requires one key")
		}
		for _, character := range part {
			if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '-' {
				return "", errors.New("invalid shortcut key")
			}
		}
		key = part
	}
	if key == "" || (!modifiers["mod"] && !modifiers["meta"] && !modifiers["control"] && !modifiers["alt"]) {
		return "", errors.New("shortcut requires a modifier and key")
	}
	if modifiers["mod"] && (modifiers["meta"] || modifiers["control"]) {
		return "", errors.New("mod cannot be combined with meta or control")
	}
	ordered := make([]string, 0, 6)
	for _, modifier := range []string{"mod", "meta", "control", "shift", "alt"} {
		if modifiers[modifier] {
			ordered = append(ordered, modifier)
		}
	}
	return strings.Join(append(ordered, key), "+"), nil
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func uniqueNonEmpty(values []string, label string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("configuration: %s cannot contain an empty value", label)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("configuration: %s contains duplicate %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validThinking(value ThinkingLevel) bool {
	switch value {
	case ThinkingNone, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingMax:
		return true
	default:
		return false
	}
}

func validApproval(value ApprovalMode) bool {
	return value == ApprovalNever || value == ApprovalOnRequest || value == ApprovalAlways
}

func validQueue(value QueueMode) bool { return value == QueueOneAtATime || value == QueueAll }

func validTrust(value TrustMode) bool {
	return value == TrustDenied || value == TrustPrompt || value == TrustTrusted
}
