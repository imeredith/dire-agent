package openrouter

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the root of OpenRouter's public API.
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	// DefaultResponsesURL is the stateless OpenRouter Responses endpoint.
	DefaultResponsesURL = DefaultBaseURL + "/responses"

	providerName        = "openrouter"
	maximumErrorBody    = 1 << 20
	initialRetryBackoff = 200 * time.Millisecond
	maximumAttempts     = 3
)

var ErrNotAuthenticated = errors.New("openrouter: API key is required; set Config.APIKey or OPENROUTER_API_KEY")

// Config controls the OpenRouter HTTP provider.
type Config struct {
	// APIKey defaults to OPENROUTER_API_KEY when empty.
	APIKey string
	// BaseURL defaults to https://openrouter.ai/api/v1. The provider appends
	// /responses and /models as appropriate.
	BaseURL string
	// DefaultModel is used when SessionOptions.Model is empty. When both are
	// empty, OpenRouter uses the API key's configured default model.
	DefaultModel string
	// HTTPReferer and AppTitle set OpenRouter's optional app-attribution
	// headers. Title is a deprecated alias for AppTitle.
	HTTPReferer string
	AppTitle    string
	Title       string
	HTTPClient  *http.Client
	// AllowUnsafeEndpoint must be true when BaseURL is overridden. An
	// override receives the API key, so it must always be explicit.
	AllowUnsafeEndpoint bool
}

func resolveConfig(config Config) (Config, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	if config.APIKey == "" {
		config.APIKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	}
	if config.APIKey == "" {
		return Config{}, ErrNotAuthenticated
	}

	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	if !config.AllowUnsafeEndpoint && config.BaseURL != DefaultBaseURL {
		return Config{}, errors.New("openrouter: endpoint override requires AllowUnsafeEndpoint because the API key will be sent to it")
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Config{}, fmt.Errorf("openrouter: invalid BaseURL %q", config.BaseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Config{}, fmt.Errorf("openrouter: invalid BaseURL scheme %q", parsed.Scheme)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, fmt.Errorf("openrouter: BaseURL must not contain user info, a query, or a fragment")
	}

	config.DefaultModel = strings.TrimSpace(config.DefaultModel)
	config.HTTPReferer = strings.TrimSpace(config.HTTPReferer)
	config.AppTitle = strings.TrimSpace(config.AppTitle)
	if config.AppTitle == "" {
		config.AppTitle = strings.TrimSpace(config.Title)
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	return config, nil
}
