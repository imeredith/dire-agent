// Package openrouter implements the stateless OpenRouter Responses API.
package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dire-kiwi/dire-agent/agent"
)

// APIError describes an HTTP or streamed error returned by OpenRouter.
type APIError struct {
	StatusCode int
	Code       string
	ErrorType  string
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	label := e.ErrorType
	if label == "" {
		label = e.Code
	}
	if label != "" {
		return fmt.Sprintf("openrouter API error (%s): %s", label, e.Message)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("openrouter API error (HTTP %d): %s", e.StatusCode, e.Message)
	}
	return "openrouter API error: " + e.Message
}

// ModelInfo is the provider-neutral subset of an OpenRouter model catalog
// entry needed by dire-agent.
type ModelInfo struct {
	ID            string `json:"id"`
	ContextLength int64  `json:"context_length"`
}

// Provider calls OpenRouter directly over HTTP.
type Provider struct {
	client       *http.Client
	baseURL      string
	apiKey       string
	defaultModel string
	httpReferer  string
	appTitle     string
}

var _ agent.StatefulProvider = (*Provider)(nil)

// New constructs an OpenRouter provider. It performs no network calls.
func New(_ context.Context, config Config) (*Provider, error) {
	resolved, err := resolveConfig(config)
	if err != nil {
		return nil, err
	}
	return &Provider{
		client: resolved.HTTPClient, baseURL: resolved.BaseURL,
		apiKey: resolved.APIKey, defaultModel: resolved.DefaultModel,
		httpReferer: resolved.HTTPReferer, appTitle: resolved.AppTitle,
	}, nil
}

// OpenSession creates an in-memory conversation. Every turn resends the full
// Responses item history because OpenRouter's Responses API is stateless.
func (p *Provider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	id, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("openrouter: create session id: %w", err)
	}
	return p.newSession(options, id, nil), nil
}

// OpenSessionFromState restores a conversation snapshot returned by State.
func (p *Provider) OpenSessionFromState(_ context.Context, options agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if state.Provider != "" && state.Provider != providerName {
		return nil, fmt.Errorf("openrouter: cannot restore provider state %q", state.Provider)
	}
	var history []json.RawMessage
	if len(state.Data) != 0 && string(state.Data) != "null" {
		if err := json.Unmarshal(state.Data, &history); err != nil {
			return nil, fmt.Errorf("openrouter: decode session state: %w", err)
		}
	}
	id := strings.TrimSpace(state.ID)
	if id == "" {
		var err error
		id, err = randomID()
		if err != nil {
			return nil, fmt.Errorf("openrouter: create session id: %w", err)
		}
	}
	return p.newSession(options, id, history), nil
}

func (p *Provider) newSession(options agent.SessionOptions, id string, history []json.RawMessage) *session {
	model := strings.TrimSpace(options.Model)
	if model == "" {
		model = p.defaultModel
	}
	return &session{
		provider: p, id: id, model: model,
		instructions: options.Instructions, history: cloneRawMessages(history),
	}
}

// ListModels returns the current OpenRouter model IDs and context lengths.
// It is opt-in and is never called while constructing a Provider.
func (p *Provider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	response, err := p.send(ctx, http.MethodGet, "/models", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var envelope struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("openrouter: decode models response: %w", err)
	}
	return envelope.Data, nil
}

func (p *Provider) validate() error {
	if p == nil || p.client == nil || strings.TrimSpace(p.apiKey) == "" || strings.TrimSpace(p.baseURL) == "" {
		return errors.New("openrouter: provider is not initialized")
	}
	return nil
}

// Close releases idle connections owned by the configured HTTP client.
func (p *Provider) Close() error {
	if p != nil && p.client != nil {
		p.client.CloseIdleConnections()
	}
	return nil
}
