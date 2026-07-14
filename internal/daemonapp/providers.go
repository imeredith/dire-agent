package daemonapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/provider/codex"
	"github.com/dire-kiwi/dire-agent/provider/openrouter"
)

const (
	codexProviderName      = "codex"
	openRouterProviderName = "openrouter"
)

type modelProviderOptions struct {
	Name             string
	DefaultModel     string
	CodexAuthFile    string
	OpenRouterAPIKey string
}

func normalizeModelProvider(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", codexProviderName:
		return codexProviderName, nil
	case openRouterProviderName:
		return openRouterProviderName, nil
	default:
		return "", fmt.Errorf("unsupported model provider %q (supported: codex, openrouter)", name)
	}
}

func defaultModelForProvider(provider string) string {
	if provider == openRouterProviderName {
		return "openrouter/auto"
	}
	return "gpt-5.6"
}

func validateProviderModel(provider, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	if provider == openRouterProviderName && !qualifiedOpenRouterModel(model) {
		return fmt.Errorf("OpenRouter model %q must be an organization-qualified slug such as openrouter/auto or anthropic/claude-sonnet-4.6", model)
	}
	return nil
}

func qualifiedOpenRouterModel(model string) bool {
	parts := strings.Split(strings.TrimSpace(model), "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return false
		}
	}
	return true
}

func newModelProvider(ctx context.Context, options modelProviderOptions) (agent.StatefulProvider, error) {
	name, err := normalizeModelProvider(options.Name)
	if err != nil {
		return nil, err
	}
	if err := validateProviderModel(name, options.DefaultModel); err != nil {
		return nil, err
	}
	switch name {
	case codexProviderName:
		return codex.New(ctx, codex.Config{
			AuthFile: options.CodexAuthFile, DefaultModel: options.DefaultModel,
		})
	case openRouterProviderName:
		return openrouter.New(ctx, openrouter.Config{
			APIKey: options.OpenRouterAPIKey, DefaultModel: options.DefaultModel,
			AppTitle: "Dire Agent",
		})
	default:
		panic("unreachable model provider")
	}
}
