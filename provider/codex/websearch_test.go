package codex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/websearch"
)

func TestWebSearchUsesHostedToolAndExtractsCitations(t *testing.T) {
	t.Parallel()
	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	var sessionMu sync.Mutex
	var sessionID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer access-token" || request.Header.Get("ChatGPT-Account-ID") != "account-123" {
			t.Errorf("authentication headers = %#v", request.Header)
		}
		if request.Header.Get("x-openai-internal-codex-responses-lite") != "" {
			t.Error("hosted web search incorrectly used Responses Lite")
		}
		requestSessionID := request.Header.Get("session-id")
		sessionMu.Lock()
		sessionID = requestSessionID
		sessionMu.Unlock()
		if requestSessionID == "" || request.Header.Get("thread-id") != requestSessionID {
			t.Errorf("search session headers = %#v", request.Header)
		}

		var body webSearchResponsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if body.Model != "gpt-5.4" || !body.Stream || body.Store || body.ToolChoice != "required" || !body.ParallelToolCalls {
			t.Errorf("request settings = %#v", body)
		}
		if body.PromptCacheKey != requestSessionID || !reflect.DeepEqual(body.Include, []string{"web_search_call.action.sources"}) {
			t.Errorf("request cache/include = %q / %#v", body.PromptCacheKey, body.Include)
		}
		if len(body.Tools) != 1 || body.Tools[0].Type != "web_search" || body.Tools[0].SearchContextSize != "medium" || body.Tools[0].Filters == nil {
			t.Errorf("hosted tools = %#v", body.Tools)
		} else if !reflect.DeepEqual(body.Tools[0].Filters.AllowedDomains, []string{"openai.com"}) ||
			!reflect.DeepEqual(body.Tools[0].Filters.BlockedDomains, []string{"reddit.com"}) {
			t.Errorf("domain filters = %#v", body.Tools[0].Filters)
		}
		for _, phrase := range []string{"past week", "around 3 distinct sources", "untrusted evidence", "Only use sources from: openai.com", "Do not use sources from: reddit.com"} {
			if !strings.Contains(body.Instructions, phrase) {
				t.Errorf("instructions missing %q: %q", phrase, body.Instructions)
			}
		}
		if len(body.Input) != 1 {
			t.Errorf("input count = %d", len(body.Input))
		} else {
			var input struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			_ = json.Unmarshal(body.Input[0], &input)
			if input.Role != "user" || len(input.Content) != 1 || input.Content[0].Type != "input_text" || input.Content[0].Text != "latest release" {
				t.Errorf("search input = %#v", input)
			}
		}

		answer := "The latest release is available from the primary source."
		writeSSE(t, writer,
			map[string]any{"type": "response.output_item.done", "item": map[string]any{
				"type": "web_search_call",
				"action": map[string]any{"sources": []map[string]string{
					{"url": "https://example.com/article?x=1&utm_source=chatgpt.com", "title": "Duplicate source"},
					{"url": "https://source.test/page", "title": "Second source"},
				}},
			}},
			map[string]any{"type": "response.output_item.done", "item": map[string]any{
				"type": "message", "role": "assistant", "phase": "final_answer",
				"content": []map[string]any{{
					"type": "output_text", "text": answer,
					"annotations": []map[string]any{{
						"type": "url_citation", "url": "https://example.com/article?utm_source=openai&x=1",
						"title": "Primary source", "start_index": 4, "end_index": 18,
					}},
				}},
			}},
			map[string]any{"type": "response.completed", "response": map[string]any{
				"id": "search-response-1", "context_window": 128_000,
				"usage": map[string]any{"input_tokens": 40, "output_tokens": 20, "total_tokens": 60},
			}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, authFile, server)
	result, err := provider.Search(context.Background(), websearch.Request{
		Query: "latest release", NumResults: 3, RecencyFilter: "week",
		AllowedDomains: []string{" OpenAI.COM "}, BlockedDomains: []string{" REDDIT.com "},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionMu.Lock()
	capturedSessionID := sessionID
	sessionMu.Unlock()
	if result.Answer != "The latest release is available from the primary source." || result.SessionID != capturedSessionID || result.TurnID != "search-response-1" {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Usage.TotalTokens != 60 || result.Usage.ContextWindow != 128_000 {
		t.Fatalf("result usage = %#v", result.Usage)
	}
	wantCitations := []websearch.Citation{
		{Title: "Primary source", URL: "https://example.com/article?x=1", Snippet: "The latest release is available from the primary source."},
		{Title: "Second source", URL: "https://source.test/page"},
	}
	if !reflect.DeepEqual(result.Citations, wantCitations) {
		t.Fatalf("citations = %#v, want %#v", result.Citations, wantCitations)
	}
}

func TestWebSearchSessionsAreEphemeral(t *testing.T) {
	t.Parallel()
	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	var calls atomic.Int32
	var mu sync.Mutex
	var headerIDs, cacheKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body webSearchResponsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		mu.Lock()
		headerIDs = append(headerIDs, request.Header.Get("session-id"))
		cacheKeys = append(cacheKeys, body.PromptCacheKey)
		mu.Unlock()
		number := calls.Add(1)
		if len(body.Input) != 1 {
			t.Errorf("search %d input count = %d", number, len(body.Input))
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.output_item.done", "item": map[string]any{
				"type": "message", "role": "assistant", "phase": "final_answer",
				"content": []map[string]string{{"type": "output_text", "text": "answer"}},
			}},
			map[string]any{"type": "response.completed", "response": map[string]string{"id": "response"}},
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, authFile, server)
	opened, err := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	parentSession := opened.(*session)
	parentSession.mu.Lock()
	parentSession.history = []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"parent turn"}]}`),
		json.RawMessage(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"parent answer"}]}`),
	}
	parentSession.mu.Unlock()
	parent := agent.StatefulSession(parentSession)
	before, err := parent.State()
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.Search(context.Background(), websearch.Request{Query: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Search(context.Background(), websearch.Request{Query: "second"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := parent.State()
	if err != nil {
		t.Fatal(err)
	}
	if before.ID != after.ID || string(before.Data) != string(after.Data) {
		t.Fatalf("parent state changed: before=%#v after=%#v", before, after)
	}
	if first.SessionID == "" || second.SessionID == "" || first.SessionID == second.SessionID {
		t.Fatalf("search session IDs = %q / %q", first.SessionID, second.SessionID)
	}
	if first.Usage.ContextWindow != webSearchContextWindow || second.Usage.ContextWindow != webSearchContextWindow {
		t.Fatalf("search context windows = %d / %d", first.Usage.ContextWindow, second.Usage.ContextWindow)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(headerIDs, []string{first.SessionID, second.SessionID}) || !reflect.DeepEqual(cacheKeys, headerIDs) {
		t.Fatalf("header/cache IDs = %#v / %#v", headerIDs, cacheKeys)
	}
}

func TestWebSearchRejectsEmptyHostedResponse(t *testing.T) {
	t.Parallel()
	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeSSE(t, writer, map[string]any{
			"type": "response.completed", "response": map[string]string{"id": "empty-response"},
		})
	}))
	defer server.Close()
	provider := newTestProvider(t, authFile, server)
	if _, err := provider.Search(context.Background(), websearch.Request{Query: "find something"}); err == nil ||
		!strings.Contains(err.Error(), "returned no answer or citations") {
		t.Fatalf("empty response error = %v", err)
	}
}

func TestWebSearchUsesConfiguredSearchModel(t *testing.T) {
	t.Parallel()
	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body webSearchResponsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if body.Model != "search-model" {
			t.Errorf("search model = %q", body.Model)
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.output_item.done", "item": map[string]any{
				"type": "message", "role": "assistant", "phase": "final_answer",
				"content": []map[string]string{{"type": "output_text", "text": "answer"}},
			}},
			map[string]any{"type": "response.completed", "response": map[string]string{"id": "response"}},
		)
	}))
	defer server.Close()
	provider, err := New(context.Background(), Config{
		BaseURL: server.URL, RefreshURL: server.URL + "/oauth/token", AuthFile: authFile,
		HTTPClient: server.Client(), AllowUnsafeEndpoint: true, WebSearchModel: "search-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if _, err := provider.Search(context.Background(), websearch.Request{Query: "model override"}); err != nil {
		t.Fatal(err)
	}
}

func TestWebSearchRejectsInvalidDirectRequests(t *testing.T) {
	t.Parallel()
	provider := &Provider{credentials: &credentialStore{}}
	for _, request := range []websearch.Request{
		{},
		{Query: "x", NumResults: 21},
		{Query: "x", RecencyFilter: "hour"},
	} {
		if _, err := provider.Search(context.Background(), request); err == nil {
			t.Fatalf("invalid request accepted: %#v", request)
		}
	}
}
