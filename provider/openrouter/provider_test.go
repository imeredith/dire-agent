package openrouter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
)

func TestNewResolvesAPIKeyAndDefaultsWithoutNetwork(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", " environment-key ")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("New made an unexpected network request")
		return nil, errors.New("unexpected request")
	})}
	provider, err := New(context.Background(), Config{HTTPClient: client})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if provider.apiKey != "environment-key" || provider.baseURL != DefaultBaseURL {
		t.Fatalf("resolved provider = key:%q base:%q", provider.apiKey, provider.baseURL)
	}
	if DefaultResponsesURL != "https://openrouter.ai/api/v1/responses" {
		t.Fatalf("DefaultResponsesURL = %q", DefaultResponsesURL)
	}

	provider, err = New(context.Background(), Config{APIKey: " explicit-key ", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if provider.apiKey != "explicit-key" {
		t.Fatalf("explicit API key was not preferred: %q", provider.apiKey)
	}
}

func TestNewRequiresAPIKeyAndSafeEndpointOptIn(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	if _, err := New(context.Background(), Config{}); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("missing API key error = %v", err)
	}
	_, err := New(context.Background(), Config{APIKey: "key", BaseURL: "http://127.0.0.1:1234"})
	if err == nil || !strings.Contains(err.Error(), "AllowUnsafeEndpoint") {
		t.Fatalf("unsafe endpoint error = %v", err)
	}
	_, err = New(context.Background(), Config{
		APIKey: "key", BaseURL: "ftp://example.com", AllowUnsafeEndpoint: true,
	})
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("invalid scheme error = %v", err)
	}
}

func TestResponsesRequestHeadersStreamingAndUsage(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/responses" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("HTTP-Referer"); got != "https://dire.example" {
			t.Errorf("HTTP-Referer = %q", got)
		}
		if got := request.Header.Get("X-OpenRouter-Title"); got != "Dire Agent" {
			t.Errorf("X-OpenRouter-Title = %q", got)
		}
		if got := request.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept = %q", got)
		}

		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if body.Model != "openai/test-model" || body.Instructions != "Be exact." {
			t.Errorf("model/instructions = %q/%q", body.Model, body.Instructions)
		}
		if !body.Stream || body.Store || body.SessionID == "" || len(body.Input) != 1 {
			t.Errorf("request flags/session/input = %#v", body)
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.content_part.delta", "response_id": "resp-1", "delta": "streamed"},
			map[string]any{"type": "response.output_item.done", "output_index": 0, "item": map[string]any{
				"type": "message", "id": "msg-1", "status": "completed", "role": "assistant",
				"content": []map[string]string{{"type": "output_text", "text": "final answer"}},
			}},
			map[string]any{"type": "response.done", "response": map[string]any{
				"id": "resp-1", "model": "openai/test-model", "context_window": 131072,
				"usage": map[string]any{
					"input_tokens": 12, "output_tokens": 5, "total_tokens": 17,
					"input_tokens_details": map[string]any{"cached_tokens": 7},
				},
			}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, server, Config{
		DefaultModel: "openai/default", HTTPReferer: "https://dire.example", AppTitle: "Dire Agent",
	})
	session, err := provider.OpenSession(context.Background(), agent.SessionOptions{
		Model: "openai/test-model", Instructions: "Be exact.",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "final answer" || result.TurnID != "resp-1" || result.Provider != providerName || result.SessionID != session.ID() {
		t.Fatalf("Run() = %#v", result)
	}
	wantUsage := agent.Usage{
		InputTokens: 12, OutputTokens: 5, CacheReadTokens: 7,
		TotalTokens: 17, ContextTokens: 17, ContextWindow: 131072,
	}
	if result.Usage != wantUsage {
		t.Fatalf("usage = %#v, want %#v", result.Usage, wantUsage)
	}
}

func TestStepStreamsReasoningAndToolCalls(t *testing.T) {
	t.Parallel()
	var requestNumber atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch requestNumber.Add(1) {
		case 1:
			if len(body.Tools) != 1 || body.Tools[0].Name != "lookup" || body.ToolChoice != "auto" {
				t.Errorf("tools = %#v, choice = %q", body.Tools, body.ToolChoice)
			}
			if body.Reasoning["effort"] != "none" {
				t.Errorf("reasoning = %#v", body.Reasoning)
			}
			writeSSE(t, writer,
				map[string]any{"type": "response.reasoning.delta", "delta": "Checking "},
				map[string]any{"type": "response.reasoning.delta", "delta": "the source."},
				map[string]any{"type": "response.reasoning.done"},
				map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{
					"type": "function_call", "id": "fc-1", "call_id": "call-1", "name": "lookup", "arguments": "",
				}},
				map[string]any{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc-1", "delta": `{"q":`},
				map[string]any{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc-1", "delta": `"x"}`},
				map[string]any{"type": "response.function_call_arguments.done", "output_index": 0, "item_id": "fc-1", "arguments": `{"q":"x"}`},
				map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-tool"}},
			)
		case 2:
			if len(body.Input) != 3 {
				t.Errorf("second input count = %d, want 3", len(body.Input))
			}
			var call struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				Arguments string `json:"arguments"`
			}
			if len(body.Input) > 1 {
				_ = json.Unmarshal(body.Input[1], &call)
			}
			if call.Type != "function_call" || call.ID != "fc-1" || call.Arguments != `{"q":"x"}` {
				t.Errorf("replayed call = %#v", call)
			}
			var output struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Output string `json:"output"`
			}
			if len(body.Input) > 2 {
				_ = json.Unmarshal(body.Input[2], &output)
			}
			if output.Type != "function_call_output" || output.CallID != "call-1" || output.Output != "found" {
				t.Errorf("tool output = %#v", output)
			}
			writeSSE(t, writer,
				map[string]any{"type": "response.output_text.delta", "delta": "done"},
				map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-final"}},
			)
		default:
			t.Errorf("unexpected request")
		}
	}))
	defer server.Close()

	provider := newTestProvider(t, server, Config{})
	opened, err := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "openai/test"})
	if err != nil {
		t.Fatal(err)
	}
	session := opened.(agent.StepSession)
	var events []agent.ModelEvent
	first, err := session.Step(context.Background(), agent.StepRequest{
		UserMessages: []string{"look it up"}, ReasoningEffort: "off",
		Tools: []agent.ToolDefinition{{
			Name: "lookup", Description: "Lookup", Parameters: json.RawMessage(`{"type":"object"}`),
		}},
		OnEvent: func(event agent.ModelEvent) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("first Step() error = %v", err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call-1" || first.ToolCalls[0].Name != "lookup" || string(first.ToolCalls[0].Arguments) != `{"q":"x"}` {
		t.Fatalf("tool calls = %#v", first.ToolCalls)
	}
	if len(events) != 4 || events[0].Type != "reasoning_delta" || events[1].Type != "reasoning_delta" || events[2].Type != "reasoning_done" || events[2].Text != "Checking the source." || events[3].Type != "item_done" {
		t.Fatalf("events = %#v", events)
	}
	second, err := session.Step(context.Background(), agent.StepRequest{
		ToolResults: []agent.ToolResult{{CallID: "call-1", Output: "found"}},
	})
	if err != nil {
		t.Fatalf("second Step() error = %v", err)
	}
	if second.Text != "done" || requestNumber.Load() != 2 {
		t.Fatalf("second result/count = %#v/%d", second, requestNumber.Load())
	}
}

func TestImageInputAndStateRestoreReplayFullItems(t *testing.T) {
	t.Parallel()
	wantImage := []byte("png bytes")
	var requestNumber atomic.Int32
	var firstSessionID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		number := requestNumber.Add(1)
		if number == 1 {
			firstSessionID = body.SessionID
			var message struct {
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL string `json:"image_url"`
				} `json:"content"`
			}
			_ = json.Unmarshal(body.Input[0], &message)
			wantURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(wantImage)
			if len(message.Content) != 2 || message.Content[0].Text != "inspect" || message.Content[1].ImageURL != wantURL {
				t.Errorf("image message = %#v", message)
			}
		} else {
			if body.SessionID != firstSessionID || len(body.Input) != 3 {
				t.Errorf("restored session/input = %q/%d, want %q/3", body.SessionID, len(body.Input), firstSessionID)
			}
			var assistant struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			_ = json.Unmarshal(body.Input[1], &assistant)
			if assistant.ID != "msg-image" || assistant.Status != "completed" {
				t.Errorf("assistant history lost required fields: %#v", assistant)
			}
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.output_item.done", "item": map[string]any{
				"type": "message", "id": fmt.Sprintf("msg-%d", number), "status": "completed", "role": "assistant",
				"content": []map[string]string{{"type": "output_text", "text": fmt.Sprintf("answer-%d", number)}},
			}},
			map[string]any{"type": "response.done", "response": map[string]any{"id": fmt.Sprintf("resp-%d", number)}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, server, Config{})
	opened, _ := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "openai/test"})
	first, err := opened.(agent.StepSession).Step(context.Background(), agent.StepRequest{
		UserMessages: []string{"inspect"},
		Images:       []agent.ImageInput{{MimeType: "image/png", Data: wantImage}},
	})
	if err != nil || first.Text != "answer-1" {
		t.Fatalf("first result/error = %#v/%v", first, err)
	}
	state, err := opened.(agent.StatefulSession).State()
	if err != nil {
		t.Fatal(err)
	}
	// The handler expects this stable ID in the replayed item.
	state.Data = json.RawMessage(strings.ReplaceAll(string(state.Data), "msg-1", "msg-image"))
	restored, err := provider.OpenSessionFromState(context.Background(), agent.SessionOptions{Model: "openai/test"}, state)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restored.Run(context.Background(), "again")
	if err != nil || result.Text != "answer-2" {
		t.Fatalf("restored result/error = %#v/%v", result, err)
	}
}

func TestStateRestoreValidation(t *testing.T) {
	t.Parallel()
	provider := newTestProvider(t, nil, Config{})
	_, err := provider.OpenSessionFromState(context.Background(), agent.SessionOptions{}, agent.SessionState{Provider: "other"})
	if err == nil || !strings.Contains(err.Error(), "cannot restore") {
		t.Fatalf("provider mismatch error = %v", err)
	}
	_, err = provider.OpenSessionFromState(context.Background(), agent.SessionOptions{}, agent.SessionState{
		Provider: providerName, Data: json.RawMessage(`{"not":"history"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "decode session state") {
		t.Fatalf("invalid state error = %v", err)
	}
}

func TestListModelsUsesAuthenticationAndAttribution(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/models" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" || request.Header.Get("HTTP-Referer") != "https://dire.example" || request.Header.Get("X-OpenRouter-Title") != "Legacy title" {
			t.Errorf("headers = %#v", request.Header)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]any{
			{"id": "openai/gpt-x", "context_length": 200000, "name": "ignored"},
			{"id": "anthropic/claude-y", "context_length": 1000000},
		}})
	}))
	defer server.Close()
	provider := newTestProvider(t, server, Config{HTTPReferer: "https://dire.example", Title: "Legacy title"})
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	want := []ModelInfo{{ID: "openai/gpt-x", ContextLength: 200000}, {ID: "anthropic/claude-y", ContextLength: 1000000}}
	if len(models) != len(want) || models[0] != want[0] || models[1] != want[1] {
		t.Fatalf("ListModels() = %#v, want %#v", models, want)
	}
}

func TestRetriesTransientResponsesAndDecodesAPIError(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if attempts.Add(1) < 3 {
			writer.Header().Set("Retry-After", "0.001")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(`{"error":{"message":"try again"}}`))
			return
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.output_text.delta", "delta": "ok"},
			map[string]any{"type": "response.done", "response": map[string]any{"id": "resp-retry"}},
		)
	}))
	defer server.Close()
	provider := newTestProvider(t, server, Config{})
	session, _ := provider.OpenSession(context.Background(), agent.SessionOptions{})
	result, err := session.Run(context.Background(), "hello")
	if err != nil || result.Text != "ok" || attempts.Load() != 3 {
		t.Fatalf("retry result/error/attempts = %#v/%v/%d", result, err, attempts.Load())
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"error_type": "invalid_request",
			"error":      map[string]any{"code": 400, "message": "bad parameter"},
		})
	}))
	defer errorServer.Close()
	errorProvider := newTestProvider(t, errorServer, Config{})
	errorSession, _ := errorProvider.OpenSession(context.Background(), agent.SessionOptions{})
	_, err = errorSession.Run(context.Background(), "hello")
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 400 || apiError.Code != "400" || apiError.ErrorType != "invalid_request" || apiError.Message != "bad parameter" {
		t.Fatalf("API error = %#v (%v)", apiError, err)
	}
}

func TestDoesNotRetryAmbiguousModelTransportFailure(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("connection reset after write")
	})}
	provider, err := New(context.Background(), Config{APIKey: "test-key", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	session, err := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "openrouter/auto"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.Run(context.Background(), "hello")
	if err == nil || attempts.Load() != 1 {
		t.Fatalf("Run error/attempts = %v/%d, want one attempt", err, attempts.Load())
	}
}

func TestStreamAliasesReasoningSummariesErrorsAndEarlyClose(t *testing.T) {
	t.Parallel()
	events := []map[string]any{
		{"type": "response.reasoning_summary_text.delta", "delta": "First."},
		{"type": "response.reasoning_summary_text.done", "text": "First."},
		{"type": "response.output_text.delta", "delta": "answer"},
		{"type": "response.output_item.done", "item": map[string]any{
			"type": "reasoning", "id": "rs-1",
			"summary": []any{"String summary", map[string]any{"type": "summary_text", "text": "Object summary"}},
		}},
		{"type": "response.completed", "response": map[string]any{
			"id": "resp-alias", "usage": map[string]any{
				"prompt_tokens": 8, "completion_tokens": 2, "cache_creation_tokens": 3,
			},
		}},
	}
	stream := encodeSSE(t, events...)
	var emitted []agent.ModelEvent
	result, err := readResponseStream(context.Background(), strings.NewReader(stream), func(event agent.ModelEvent) {
		emitted = append(emitted, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.finalText() != "answer" || result.usage.InputTokens != 8 || result.usage.OutputTokens != 2 || result.usage.CacheWriteTokens != 3 || result.usage.TotalTokens != 10 {
		t.Fatalf("stream result = %#v, text = %q", result.usage, result.finalText())
	}
	if len(emitted) < 3 || emitted[0].Type != "reasoning_delta" || emitted[1].Type != "reasoning_done" || emitted[2].Type != "text_delta" {
		t.Fatalf("emitted events = %#v", emitted)
	}

	failed := encodeSSE(t, map[string]any{
		"type": "response.failed", "response": map[string]any{
			"error_type": "provider_overloaded",
			"error":      map[string]any{"code": "server_error", "message": "overloaded"},
		},
	})
	_, err = readResponseStream(context.Background(), strings.NewReader(failed), nil)
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Code != "server_error" || apiError.ErrorType != "provider_overloaded" {
		t.Fatalf("failed event error = %#v (%v)", apiError, err)
	}
	_, err = readResponseStream(context.Background(), strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "before response.done") {
		t.Fatalf("early-close error = %v", err)
	}
}

func TestRetryWaitHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Retry-After", "60")
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider := newTestProvider(t, server, Config{})
	session, _ := provider.OpenSession(context.Background(), agent.SessionOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := session.Run(ctx, "hello")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled retry error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func newTestProvider(t *testing.T, server *httptest.Server, extra Config) *Provider {
	t.Helper()
	extra.APIKey = "test-key"
	if server != nil {
		extra.BaseURL = server.URL
		extra.HTTPClient = server.Client()
		extra.AllowUnsafeEndpoint = true
	} else if extra.HTTPClient == nil {
		extra.HTTPClient = &http.Client{}
	}
	provider, err := New(context.Background(), extra)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	return provider
}

func writeSSE(t *testing.T, writer http.ResponseWriter, events ...map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "text/event-stream")
	_, _ = writer.Write([]byte(encodeSSE(t, events...)))
}

func encodeSSE(t *testing.T, events ...map[string]any) string {
	t.Helper()
	var stream strings.Builder
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("encode SSE: %v", err)
		}
		fmt.Fprintf(&stream, "data: %s\n\n", payload)
	}
	return stream.String()
}
