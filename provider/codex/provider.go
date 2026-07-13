// Package codex implements direct HTTP access to the internal Codex Responses
// endpoint using credentials created by `codex login`.
//
// This endpoint is used by the open-source Codex client, but it is not a public,
// documented API. Callers should expect its URL, headers, and payload to change.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
)

var (
	// ErrNotAuthenticated means no usable Codex CLI login was found.
	ErrNotAuthenticated = errors.New("codex: not authenticated; run `codex login`")
	// ErrSubscriptionRequired means auth.json contains API-key credentials rather
	// than a ChatGPT subscription login.
	ErrSubscriptionRequired = errors.New("codex: ChatGPT subscription login required; run `codex login`")
)

// AccountInfo is the non-secret account metadata derived from the CLI login.
type AccountInfo struct {
	Mode string
	Plan string
}

// APIError describes an HTTP or streamed error returned by the Codex endpoint.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("codex API error (%s): %s", e.Code, e.Message)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("codex API error (HTTP %d): %s", e.StatusCode, e.Message)
	}
	return "codex API error: " + e.Message
}

// Provider calls the Codex subscription endpoint without starting Codex CLI or
// app-server.
type Provider struct {
	client          *http.Client
	baseURL         string
	defaultModel    string
	userAgent       string
	protocolVersion string
	credentials     *credentialStore
}

// New loads and validates the existing Codex CLI subscription credentials.
func New(ctx context.Context, config Config) (*Provider, error) {
	resolved, err := resolveConfig(config)
	if err != nil {
		return nil, err
	}

	store := &credentialStore{
		path: resolved.AuthFile, refreshURL: resolved.RefreshURL,
		oauthClient: resolved.OAuthClientID, client: resolved.HTTPClient,
		now: time.Now, refreshWindow: 5 * time.Minute,
	}
	if _, err := store.current(ctx); err != nil {
		return nil, err
	}

	return &Provider{
		client: resolved.HTTPClient, baseURL: strings.TrimRight(resolved.BaseURL, "/"),
		defaultModel: resolved.DefaultModel, userAgent: resolved.UserAgent,
		protocolVersion: resolved.ProtocolVersion, credentials: store,
	}, nil
}

// Account reports non-secret metadata from the current CLI credential set.
func (p *Provider) Account(ctx context.Context) (AccountInfo, error) {
	if p == nil || p.credentials == nil {
		return AccountInfo{}, errors.New("codex: provider is not initialized")
	}
	credential, err := p.credentials.current(ctx)
	if err != nil {
		return AccountInfo{}, err
	}
	return AccountInfo{Mode: "chatgpt", Plan: credential.plan}, nil
}

// OpenSession creates an in-memory conversation. Follow-up turns resend the
// accumulated Responses API items because requests use store=false.
func (p *Provider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	if p == nil || p.credentials == nil {
		return nil, errors.New("codex: provider is not initialized")
	}
	id, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("codex: create session id: %w", err)
	}
	return p.newSession(options, id, nil), nil
}

// OpenSessionFromState restores a direct Codex conversation from an opaque
// state snapshot previously returned by State.
func (p *Provider) OpenSessionFromState(_ context.Context, options agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	if p == nil || p.credentials == nil {
		return nil, errors.New("codex: provider is not initialized")
	}
	if state.Provider != "" && state.Provider != providerName {
		return nil, fmt.Errorf("codex: cannot restore provider state %q", state.Provider)
	}
	var history []json.RawMessage
	if len(state.Data) != 0 && string(state.Data) != "null" {
		if err := json.Unmarshal(state.Data, &history); err != nil {
			return nil, fmt.Errorf("codex: decode session state: %w", err)
		}
	}
	id := state.ID
	if id == "" {
		var err error
		id, err = randomID()
		if err != nil {
			return nil, fmt.Errorf("codex: create session id: %w", err)
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
		provider: p, id: id, model: model, instructions: options.Instructions,
		history: append([]json.RawMessage(nil), history...),
	}
}

// Close is present for the provider-neutral interface. Direct HTTP mode owns no
// subprocess and the shared http.Client has no mandatory close operation.
func (p *Provider) Close() error {
	if p != nil && p.client != nil {
		p.client.CloseIdleConnections()
	}
	return nil
}
