package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
)

func TestDirectResponsesRequest(t *testing.T) {
	t.Parallel()

	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			http.NotFound(writer, request)
			return
		}
		if got := request.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("ChatGPT-Account-ID"); got != "account-123" {
			t.Errorf("ChatGPT-Account-ID = %q", got)
		}
		if got := request.Header.Get("originator"); got != defaultOriginator {
			t.Errorf("originator = %q", got)
		}
		if got := request.Header.Get("Version"); got != defaultProtocolVersion {
			t.Errorf("Version = %q", got)
		}
		if request.Header.Get("session-id") == "" || request.Header.Get("thread-id") == "" {
			t.Error("session routing headers are missing")
		}
		if request.Header.Get("x-openai-internal-codex-responses-lite") != "" {
			t.Error("Responses Lite header set for an unrelated provider model")
		}

		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if body.Model != "model-a" || body.Instructions != "Be concise." {
			t.Errorf("request model/instructions = %q/%q", body.Model, body.Instructions)
		}
		if !body.Stream || body.Store || len(body.Input) != 1 {
			t.Errorf("request flags/input = stream:%v store:%v input:%d", body.Stream, body.Store, len(body.Input))
		}

		writeSSE(t, writer,
			map[string]any{
				"type": "response.output_item.done",
				"item": map[string]any{
					"id": "msg-commentary", "type": "message", "role": "assistant", "phase": "commentary",
					"content": []map[string]string{{"type": "output_text", "text": "working"}},
				},
			},
			map[string]any{"type": "response.output_text.delta", "delta": "streamed answer"},
			map[string]any{
				"type": "response.output_item.done",
				"item": map[string]any{
					"id": "msg-final", "type": "message", "role": "assistant", "phase": "final_answer",
					"content": []map[string]string{{"type": "output_text", "text": "authoritative answer"}},
				},
			},
			map[string]any{"type": "response.completed", "response": map[string]any{
				"id": "response-1", "context_window": 32_768,
				"usage": map[string]any{
					"input_tokens": 120, "output_tokens": 30, "total_tokens": 150,
					"input_tokens_details": map[string]any{"cached_tokens": 80, "cache_write_tokens": 24},
				},
			}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, authFile, server)
	account, err := provider.Account(context.Background())
	if err != nil {
		t.Fatalf("Account() error = %v", err)
	}
	if account.Mode != "chatgpt" || account.Plan != "plus" {
		t.Fatalf("Account() = %#v", account)
	}

	session, err := provider.OpenSession(context.Background(), agent.SessionOptions{
		Model:        "model-a",
		Instructions: "Be concise.",
	})
	if err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	result, err := session.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "authoritative answer" || result.TurnID != "response-1" {
		t.Fatalf("Run() = %#v", result)
	}
	if result.Provider != providerName || result.SessionID != session.ID() {
		t.Fatalf("Run() metadata = %#v", result)
	}
	wantUsage := agent.Usage{
		InputTokens: 120, OutputTokens: 30, CacheReadTokens: 80,
		CacheWriteTokens: 24, TotalTokens: 150,
		ContextTokens: 150, ContextWindow: 32_768,
	}
	if result.Usage != wantUsage {
		t.Fatalf("Run().Usage = %#v, want %#v", result.Usage, wantUsage)
	}
}

func TestSessionResendsConversationHistory(t *testing.T) {
	t.Parallel()

	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	var requestNumber atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		number := requestNumber.Add(1)
		wantItems := 1
		if number == 2 {
			wantItems = 3
		}
		if len(body.Input) != wantItems {
			t.Errorf("request %d input items = %d, want %d", number, len(body.Input), wantItems)
		}
		if body.PromptCacheKey == "" {
			t.Errorf("request %d prompt cache key is empty", number)
		}
		writeSSE(t, writer,
			map[string]any{
				"type": "response.output_item.done",
				"item": map[string]any{
					"id": fmt.Sprintf("message-%d", number), "type": "message", "role": "assistant",
					"content": []map[string]string{{"type": "output_text", "text": fmt.Sprintf("answer-%d", number)}},
				},
			},
			map[string]any{"type": "response.completed", "response": map[string]string{"id": fmt.Sprintf("response-%d", number)}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, authFile, server)
	session, err := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Run(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	if requestNumber.Load() != 2 {
		t.Fatalf("request count = %d, want 2", requestNumber.Load())
	}
}

func TestAgenticToolCallAndResult(t *testing.T) {
	t.Parallel()

	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	var requestNumber atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch requestNumber.Add(1) {
		case 1:
			if len(body.Tools) != 1 || body.Tools[0].Name != "lookup" {
				t.Errorf("tools = %#v", body.Tools)
			}
			writeSSE(t, writer,
				map[string]any{"type": "response.output_item.done", "item": map[string]any{
					"type": "function_call", "call_id": "call-1", "name": "lookup", "arguments": `{"q":"x"}`,
				}},
				map[string]any{"type": "response.completed", "response": map[string]string{"id": "response-1"}},
			)
		case 2:
			if len(body.Input) != 3 {
				t.Errorf("second request input count = %d, want 3", len(body.Input))
			}
			var output struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Output string `json:"output"`
			}
			if len(body.Input) >= 3 {
				_ = json.Unmarshal(body.Input[2], &output)
			}
			if output.Type != "function_call_output" || output.CallID != "call-1" || output.Output != "found" {
				t.Errorf("tool output = %#v", output)
			}
			writeSSE(t, writer,
				map[string]any{"type": "response.output_item.done", "item": map[string]any{
					"id": "message-2", "type": "message", "role": "assistant", "phase": "final_answer",
					"content": []map[string]string{{"type": "output_text", "text": "done"}},
				}},
				map[string]any{"type": "response.completed", "response": map[string]string{"id": "response-2"}},
			)
		default:
			t.Errorf("unexpected request")
		}
	}))
	defer server.Close()

	provider := newTestProvider(t, authFile, server)
	opened, err := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	session := opened.(agent.StepSession)
	first, err := session.Step(context.Background(), agent.StepRequest{
		UserMessages: []string{"look it up"},
		Tools:        []agent.ToolDefinition{{Name: "lookup", Description: "Lookup a value", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call-1" || first.ToolCalls[0].Name != "lookup" {
		t.Fatalf("tool calls = %#v", first.ToolCalls)
	}
	second, err := session.Step(context.Background(), agent.StepRequest{
		ToolResults: []agent.ToolResult{{CallID: "call-1", Output: "found"}},
		Tools:       []agent.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Text != "done" || requestNumber.Load() != 2 {
		t.Fatalf("second step/count = %#v/%d", second, requestNumber.Load())
	}
}

func TestUnauthorizedRefreshesAndPersistsCredentials(t *testing.T) {
	t.Parallel()

	authFile := writeTestAuth(t, "old-access", "old-refresh", "account-123", "plus", time.Now().Add(time.Hour))
	var responseRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/oauth/token":
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode refresh: %v", err)
			}
			if body["refresh_token"] != "old-refresh" || body["grant_type"] != "refresh_token" {
				t.Errorf("refresh body = %#v", body)
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"access_token":  "new-access",
				"refresh_token": "new-refresh",
				"id_token":      testJWT("pro", "account-123", time.Now().Add(time.Hour)),
			})
		case "/responses":
			responseRequests.Add(1)
			if request.Header.Get("Authorization") == "Bearer old-access" {
				writer.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"error": map[string]string{"code": "invalid_token", "message": "expired"},
				})
				return
			}
			if got := request.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Errorf("Authorization after refresh = %q", got)
			}
			writeSSE(t, writer,
				map[string]any{"type": "response.output_text.delta", "delta": "ok"},
				map[string]any{"type": "response.completed", "response": map[string]string{"id": "response-1"}},
			)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	provider, err := New(context.Background(), Config{
		BaseURL:             server.URL,
		RefreshURL:          server.URL + "/oauth/token",
		OAuthClientID:       "test-client",
		AuthFile:            authFile,
		HTTPClient:          server.Client(),
		AllowUnsafeEndpoint: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session, _ := provider.OpenSession(context.Background(), agent.SessionOptions{})
	result, err := session.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "ok" || responseRequests.Load() != 2 {
		t.Fatalf("result/count = %#v/%d", result, responseRequests.Load())
	}

	contents, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Future string `json:"future"`
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			FutureToken  string `json:"future_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(contents, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Tokens.AccessToken != "new-access" || saved.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("saved tokens were not refreshed: %#v", saved.Tokens)
	}
	if saved.Future != "keep" || saved.Tokens.FutureToken != "keep-token" {
		t.Fatalf("unknown auth fields were not preserved: %#v", saved)
	}
}

func TestAuthenticationErrors(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.json")
	_, err := New(context.Background(), Config{AuthFile: missing})
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("missing auth error = %v", err)
	}

	apiKeyFile := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(apiKeyFile, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"test-api-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = New(context.Background(), Config{AuthFile: apiKeyFile})
	if !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("API key auth error = %v", err)
	}
}

func TestEndpointOverrideRequiresExplicitOptIn(t *testing.T) {
	t.Parallel()

	_, err := resolveConfig(Config{BaseURL: "http://127.0.0.1:1234"})
	if err == nil || !strings.Contains(err.Error(), "AllowUnsafeEndpoint") {
		t.Fatalf("resolveConfig() error = %v", err)
	}
}

func TestResponseFailedEvent(t *testing.T) {
	t.Parallel()

	stream := strings.NewReader("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"slow down\"}}}\n\n")
	_, err := readResponseStream(context.Background(), stream, nil)
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Code != "rate_limit_exceeded" {
		t.Fatalf("readResponseStream() error = %v", err)
	}
}

func TestResponseStreamEmitsReasoningSummary(t *testing.T) {
	t.Parallel()
	events := []map[string]any{
		{"type": "response.reasoning_summary_text.delta", "delta": "Inspecting "},
		{"type": "response.reasoning_summary_text.delta", "delta": "the request."},
		{"type": "response.reasoning_summary_text.done", "text": "Inspecting the request."},
		{"type": "response.output_text.delta", "delta": "Done."},
		{"type": "response.completed", "response": map[string]any{"id": "response-reasoning"}},
	}
	var stream strings.Builder
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&stream, "data: %s\n\n", payload)
	}
	var modelEvents []agent.ModelEvent
	result, err := readResponseStream(context.Background(), strings.NewReader(stream.String()), func(event agent.ModelEvent) {
		modelEvents = append(modelEvents, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.finalText() != "Done." {
		t.Fatalf("final text = %q", result.finalText())
	}
	if len(modelEvents) != 4 || modelEvents[0].Type != "reasoning_delta" ||
		modelEvents[1].Type != "reasoning_delta" || modelEvents[2].Type != "reasoning_done" ||
		modelEvents[2].Text != "Inspecting the request." || modelEvents[3].Type != "text_delta" {
		t.Fatalf("model events = %#v", modelEvents)
	}
}

func TestResponseStreamFindsReasoningSummaryInCompletedItem(t *testing.T) {
	t.Parallel()
	item, _ := json.Marshal(map[string]any{
		"type":    "reasoning",
		"summary": []map[string]string{{"type": "summary_text", "text": "Checked the files."}},
	})
	var got []agent.ModelEvent
	payload, _ := json.Marshal(map[string]any{"type": "response.output_item.done", "item": json.RawMessage(item)})
	completed, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "response-item"}})
	stream := fmt.Sprintf("data: %s\n\ndata: %s\n\n", payload, completed)
	if _, err := readResponseStream(context.Background(), strings.NewReader(stream), func(event agent.ModelEvent) {
		got = append(got, event)
	}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range got {
		if event.Type == "reasoning_done" && event.Text == "Checked the files." {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasoning summary event missing: %#v", got)
	}
}

func TestResponseUsageCacheCreationVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		usage      map[string]any
		cacheRead  int64
		cacheWrite int64
	}{
		{
			name: "top-level Codex cached input tokens",
			usage: map[string]any{
				"input_tokens": 210, "output_tokens": 12, "cached_input_tokens": 128,
			},
			cacheRead: 128,
		},
		{
			name: "top-level cache creation input tokens",
			usage: map[string]any{
				"input_tokens": 200, "output_tokens": 25,
				"cache_read_input_tokens": 120, "cache_creation_input_tokens": 64,
			},
			cacheRead: 120, cacheWrite: 64,
		},
		{
			name: "nested cache creation duration buckets",
			usage: map[string]any{
				"input_tokens": 300, "output_tokens": 40, "total_tokens": 340,
				"input_tokens_details": map[string]any{
					"cached_tokens": 160,
					"cache_creation": map[string]any{
						"ephemeral_5m_input_tokens": 32,
						"ephemeral_1h_input_tokens": 16,
					},
				},
			},
			cacheRead: 160, cacheWrite: 48,
		},
		{
			name: "scalar cache creation alias",
			usage: map[string]any{
				"input_tokens": 75, "output_tokens": 5,
				"cached_tokens": 40, "cache_creation": 20,
			},
			cacheRead: 40, cacheWrite: 20,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id": "response-usage", "model": "gpt-5.6-sol", "usage": test.usage,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			streamed, err := readResponseStream(context.Background(), strings.NewReader("data: "+string(payload)+"\n\n"), nil)
			if err != nil {
				t.Fatalf("readResponseStream() error = %v", err)
			}
			if streamed.usage.CacheReadTokens != test.cacheRead || streamed.usage.CacheWriteTokens != test.cacheWrite {
				t.Fatalf("cache usage = %#v, want read/write %d/%d", streamed.usage, test.cacheRead, test.cacheWrite)
			}
			if streamed.usage.ContextTokens != streamed.usage.InputTokens+streamed.usage.OutputTokens {
				t.Fatalf("context tokens = %d, want current input + output", streamed.usage.ContextTokens)
			}
			if streamed.usage.ContextWindow != 372_000 {
				t.Fatalf("context window = %d, want 372000", streamed.usage.ContextWindow)
			}
			if streamed.usage.TotalTokens != streamed.usage.InputTokens+streamed.usage.OutputTokens {
				t.Fatalf("total tokens = %d, want input + output fallback", streamed.usage.TotalTokens)
			}
		})
	}
}

func TestResponsesLiteRequestForLuna(t *testing.T) {
	t.Parallel()

	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("x-openai-internal-codex-responses-lite"); got != "true" {
			t.Errorf("Responses Lite header = %q", got)
		}
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "gpt-5.6-luna" || body.Instructions != "" || len(body.Tools) != 0 {
			t.Errorf("Luna request model/instructions/tools = %q/%q/%d", body.Model, body.Instructions, len(body.Tools))
		}
		if body.Reasoning["context"] != "all_turns" || len(body.Input) != 3 {
			t.Errorf("Luna reasoning/input = %#v/%d", body.Reasoning, len(body.Input))
		}
		var prefix struct {
			Type  string                   `json:"type"`
			Role  string                   `json:"role"`
			Tools []responseToolDefinition `json:"tools"`
		}
		if err := json.Unmarshal(body.Input[0], &prefix); err != nil {
			t.Fatalf("decode Responses Lite prefix: %v", err)
		}
		if prefix.Type != "additional_tools" || prefix.Role != "developer" || len(prefix.Tools) != 1 || prefix.Tools[0].Name != "lookup" {
			t.Errorf("Responses Lite tool prefix = %#v", prefix)
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.output_text.delta", "delta": "ok"},
			map[string]any{"type": "response.completed", "response": map[string]any{
				"id": "response-luna", "model": "gpt-5.6-luna",
				"usage": map[string]any{"input_tokens": 40, "cached_input_tokens": 24, "output_tokens": 2},
			}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, authFile, server)
	opened, err := provider.OpenSession(context.Background(), agent.SessionOptions{
		Model: "gpt-5.6-luna", Instructions: "Be exact.",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := opened.(agent.StepSession).Step(context.Background(), agent.StepRequest{
		UserMessages: []string{"hello"},
		Tools: []agent.ToolDefinition{{
			Name: "lookup", Description: "Lookup", Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "ok" || result.Usage.CacheReadTokens != 24 || result.Usage.ContextWindow != 372_000 {
		t.Fatalf("Luna result = %#v", result)
	}
}

func TestContextWindowForModel(t *testing.T) {
	t.Parallel()

	tests := map[string]int64{
		"gpt-5.6":                 372_000,
		"gpt-5.6-sol":             372_000,
		"openai/gpt-5.6-terra":    372_000,
		"gpt-5.4":                 272_000,
		"gpt-5.4-2026-03-05":      272_000,
		"openai.gpt-5.4-pro":      272_000,
		"gpt-5.4-mini-2026-03-17": 0,
		"gpt-5.4-nano":            0,
		"provider-specific-model": 0,
	}
	for model, want := range tests {
		if got := contextWindowForModel(model); got != want {
			t.Errorf("contextWindowForModel(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestCodexSubscriptionModelAlias(t *testing.T) {
	t.Parallel()

	if got := codexSubscriptionModel("gpt-5.6"); got != "gpt-5.6-sol" {
		t.Fatalf("codexSubscriptionModel(gpt-5.6) = %q", got)
	}
	if got := codexSubscriptionModel("gpt-5.6-terra"); got != "gpt-5.6-terra" {
		t.Fatalf("concrete GPT-5.6 variant changed to %q", got)
	}
	if got := codexSubscriptionModel("provider-model"); got != "provider-model" {
		t.Fatalf("provider model changed to %q", got)
	}
}

func newTestProvider(t *testing.T, authFile string, server *httptest.Server) *Provider {
	t.Helper()
	provider, err := New(context.Background(), Config{
		BaseURL:             server.URL,
		RefreshURL:          server.URL + "/oauth/token",
		AuthFile:            authFile,
		HTTPClient:          server.Client(),
		AllowUnsafeEndpoint: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	return provider
}

func writeTestAuth(t *testing.T, accessToken, refreshToken, accountID, plan string, expires time.Time) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	document := map[string]any{
		"auth_mode": "chatgpt",
		"future":    "keep",
		"tokens": map[string]any{
			"id_token":      testJWT(plan, accountID, expires),
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"account_id":    accountID,
			"future_token":  "keep-token",
		},
		"last_refresh": time.Now().UTC().Format(time.RFC3339),
	}
	contents, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testJWT(plan, accountID string, expires time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]any{
		"exp": expires.Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type":          plan,
			"chatgpt_account_id":         accountID,
			"chatgpt_account_is_fedramp": false,
		},
	})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func writeSSE(t *testing.T, writer http.ResponseWriter, events ...map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Errorf("encode SSE: %v", err)
			return
		}
		if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
			t.Errorf("write SSE: %v", err)
			return
		}
	}
}
