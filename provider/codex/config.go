package codex

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultBaseURL         = "https://chatgpt.com/backend-api/codex"
	defaultRefreshURL      = "https://auth.openai.com/oauth/token"
	defaultOAuthClient     = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultModel           = "gpt-5.6"
	defaultProtocolVersion = "0.144.0-alpha.4"
	defaultUserAgent       = "codex_cli_rs/0.144.0-alpha.4 (dire-agent/0.1.0)"
	defaultOriginator      = "codex_cli_rs"
	providerName           = "codex-subscription-direct"
	maximumErrorBody       = 1 << 20
	initialRetryBackoff    = 200 * time.Millisecond
)

// Config controls the direct Codex HTTP client.
type Config struct {
	// BaseURL defaults to https://chatgpt.com/backend-api/codex.
	BaseURL string
	// AuthFile defaults to $CODEX_HOME/auth.json, or ~/.codex/auth.json when
	// CODEX_HOME is unset.
	AuthFile string
	// RefreshURL and OAuthClientID default to the values used by Codex CLI.
	RefreshURL    string
	OAuthClientID string
	// DefaultModel is used when SessionOptions.Model is empty.
	DefaultModel string
	UserAgent    string
	// ProtocolVersion is sent in the Version header expected by the private
	// Codex subscription endpoint. Override it when that wire contract advances.
	ProtocolVersion string
	HTTPClient      *http.Client
	// AllowUnsafeEndpoint must be true when BaseURL or RefreshURL is overridden.
	// Those endpoints receive bearer or refresh tokens, so overrides must be
	// explicit to reduce accidental credential disclosure.
	AllowUnsafeEndpoint bool
}

func resolveConfig(config Config) (Config, error) {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.RefreshURL == "" {
		config.RefreshURL = defaultRefreshURL
	}
	if config.OAuthClientID == "" {
		config.OAuthClientID = defaultOAuthClient
	}
	if config.DefaultModel == "" {
		config.DefaultModel = defaultModel
	}
	if config.UserAgent == "" {
		config.UserAgent = defaultUserAgent
	}
	if config.ProtocolVersion == "" {
		config.ProtocolVersion = defaultProtocolVersion
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	if config.AuthFile == "" {
		codexHome := os.Getenv("CODEX_HOME")
		if codexHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return Config{}, fmt.Errorf("codex: resolve home directory: %w", err)
			}
			codexHome = filepath.Join(home, ".codex")
		}
		config.AuthFile = filepath.Join(codexHome, "auth.json")
	}

	if !config.AllowUnsafeEndpoint && (config.BaseURL != defaultBaseURL || config.RefreshURL != defaultRefreshURL) {
		return Config{}, errors.New("codex: endpoint override requires AllowUnsafeEndpoint because credentials will be sent to it")
	}
	for name, value := range map[string]string{"BaseURL": config.BaseURL, "RefreshURL": config.RefreshURL} {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return Config{}, fmt.Errorf("codex: invalid %s %q", name, value)
		}
	}
	return config, nil
}
