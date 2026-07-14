package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestToolDefinitionAndDispatch(t *testing.T) {
	t.Parallel()
	searcher := &recordingSearcher{responses: []Response{{
		Answer:    "A grounded answer.",
		Citations: []Citation{{Title: "OpenAI docs", URL: "https://openai.com/docs", Snippet: "Primary source."}},
	}}}
	searchTool, err := NewTool(searcher)
	if err != nil {
		t.Fatal(err)
	}
	definition := searchTool.Definition()
	if definition.Name != Name || !json.Valid(definition.Parameters) {
		t.Fatalf("definition = %#v", definition)
	}
	for _, field := range []string{`"query"`, `"queries"`, `"numResults"`, `"recencyFilter"`, `"domainFilter"`} {
		if !strings.Contains(string(definition.Parameters), field) {
			t.Fatalf("schema is missing %s: %s", field, definition.Parameters)
		}
	}

	output, err := searchTool.Execute(context.Background(), json.RawMessage(`{
		"query":" current model ",
		"numResults":3,
		"recencyFilter":"week",
		"domainFilter":["https://OpenAI.com/research", "openai.com", "-Reddit.com"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	want := Request{
		Query: "current model", NumResults: 3, RecencyFilter: "week",
		AllowedDomains: []string{"openai.com"}, BlockedDomains: []string{"reddit.com"},
	}
	if requests := searcher.Requests(); !reflect.DeepEqual(requests, []Request{want}) {
		t.Fatalf("requests = %#v, want %#v", requests, []Request{want})
	}
	if !strings.Contains(output, "A grounded answer.") || !strings.Contains(output, "[OpenAI docs](https://openai.com/docs)") || !strings.Contains(output, "Primary source.") {
		t.Fatalf("output = %q", output)
	}
}

func TestToolBatchContinuesAfterOneSearchFails(t *testing.T) {
	t.Parallel()
	searcher := &recordingSearcher{
		responses: []Response{{}, {Answer: "second answer"}},
		errors:    []error{errors.New("provider unavailable"), nil},
	}
	searchTool, err := NewTool(searcher)
	if err != nil {
		t.Fatal(err)
	}
	output, err := searchTool.Execute(context.Background(), json.RawMessage(`{"queries":["first","second"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "## first") || !strings.Contains(output, "provider unavailable") ||
		!strings.Contains(output, "## second") || !strings.Contains(output, "second answer") {
		t.Fatalf("batch output = %q", output)
	}
	requests := searcher.Requests()
	if len(requests) != 2 || requests[0].NumResults != defaultResults {
		t.Fatalf("batch requests = %#v", requests)
	}
}

func TestToolDropsUnsafeCitationURLs(t *testing.T) {
	t.Parallel()
	searcher := &recordingSearcher{responses: []Response{{
		Answer: "safe answer",
		Citations: []Citation{
			{Title: "unsafe", URL: "javascript:alert(1)"},
			{Title: "credential", URL: "https://user:secret@example.com/private"},
			{Title: "safe", URL: "https://example.com/a_(b)"},
		},
	}}}
	searchTool, err := NewTool(searcher)
	if err != nil {
		t.Fatal(err)
	}
	output, err := searchTool.Execute(context.Background(), json.RawMessage(`{"query":"security"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "javascript:") || strings.Contains(output, "secret") || !strings.Contains(output, "https://example.com/a_%28b%29") {
		t.Fatalf("citation output = %q", output)
	}
}

func TestToolRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	tests := []string{
		`{`,
		`{} {}`,
		`{}`,
		`{"query":"","queries":["x"]}`,
		`{"query":"x","queries":["y"]}`,
		`{"queries":[]}`,
		`{"queries":["x",""]}`,
		`{"queries":["x","x"]}`,
		`{"query":"x","numResults":21}`,
		`{"query":"x","recencyFilter":"hour"}`,
		`{"query":"x","domainFilter":["-"]}`,
		`{"query":"x","domainFilter":["example.com:443"]}`,
		`{"query":"x","domainFilter":["example..com"]}`,
		`{"query":"x","domainFilter":["foo-.bar.com"]}`,
		`{"query":"x","domainFilter":["example.com","-example.com"]}`,
		`{"query":"x","unknown":true}`,
	}
	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			searchTool, err := NewTool(&recordingSearcher{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := searchTool.Execute(context.Background(), json.RawMessage(raw)); err == nil {
				t.Fatal("invalid arguments were accepted")
			}
		})
	}
}

func TestNormalizeDomainsSupportsIndependentProviderLimits(t *testing.T) {
	t.Parallel()
	values := make([]string, 0, 200)
	for index := 0; index < 100; index++ {
		values = append(values, fmt.Sprintf("allow-%d.example.com", index))
	}
	for index := 0; index < 100; index++ {
		values = append(values, fmt.Sprintf("-block-%d.example.com", index))
	}
	allowed, blocked, err := normalizeDomains(values)
	if err != nil || len(allowed) != 100 || len(blocked) != 100 {
		t.Fatalf("normalized domains = %d/%d, %v", len(allowed), len(blocked), err)
	}
}

func TestToolPropagatesCancellationAndProviderErrors(t *testing.T) {
	t.Parallel()
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		searcher := searcherFunc(func(ctx context.Context, _ Request) (Response, error) {
			close(started)
			<-ctx.Done()
			return Response{}, ctx.Err()
		})
		searchTool, err := NewTool(searcher)
		if err != nil {
			t.Fatal(err)
		}
		ctx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer timeoutCancel()
		ctx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			_, runErr := searchTool.Execute(ctx, json.RawMessage(`{"query":"wait"}`))
			done <- runErr
		}()
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("canceled search did not return")
		}
	})

	t.Run("provider error", func(t *testing.T) {
		want := errors.New("provider exploded")
		searchTool, err := NewTool(searcherFunc(func(context.Context, Request) (Response, error) {
			return Response{}, want
		}))
		if err != nil {
			t.Fatal(err)
		}
		output, err := searchTool.Execute(context.Background(), json.RawMessage(`{"query":"boom"}`))
		if !errors.Is(err, want) || output != "" {
			t.Fatalf("output/error = %q / %v", output, err)
		}
	})
}

func TestTruncateOutputBoundsAndRepairsUTF8(t *testing.T) {
	t.Parallel()
	value := strings.Repeat("a", maximumOutput/2) + string([]byte{0xff}) + strings.Repeat("世", maximumOutput)
	output := truncateOutput(value)
	if !utf8.ValidString(output) || len(output) > maximumOutput+len("\n\n[web search output truncated]") {
		t.Fatalf("truncated output length/UTF-8 = %d / %v", len(output), utf8.ValidString(output))
	}
	if !strings.HasSuffix(output, "[web search output truncated]") {
		t.Fatalf("missing truncation marker: %q", output[len(output)-64:])
	}
}

func TestCanceledBatchStillBoundsEarlierOutput(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	var calls atomic.Int32
	searcher := searcherFunc(func(ctx context.Context, _ Request) (Response, error) {
		if calls.Add(1) == 1 {
			return Response{Answer: strings.Repeat("x", maximumOutput+1024)}, nil
		}
		close(started)
		<-ctx.Done()
		return Response{}, ctx.Err()
	})
	searchTool, err := NewTool(searcher)
	if err != nil {
		t.Fatal(err)
	}
	ctx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer timeoutCancel()
	ctx, cancel := context.WithCancel(ctx)
	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, runErr := searchTool.Execute(ctx, json.RawMessage(`{"queries":["large","wait"]}`))
		done <- result{output: output, err: runErr}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) || len(got.output) > maximumOutput+len("\n\n[web search output truncated]") {
			t.Fatalf("canceled batch output/error = %d / %v", len(got.output), got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled batch did not return")
	}
}

func TestToolEnforcesPerRunSearchBudget(t *testing.T) {
	t.Parallel()
	searchTool, err := NewTool(&recordingSearcher{responses: make([]Response, searchesPerRun)})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < searchesPerRun; index++ {
		query := fmt.Sprintf(`{"query":"query-%d"}`, index)
		if _, err := searchTool.Execute(context.Background(), json.RawMessage(query)); err != nil {
			t.Fatalf("search %d: %v", index, err)
		}
	}
	if _, err := searchTool.Execute(context.Background(), json.RawMessage(`{"query":"one-too-many"}`)); err == nil ||
		!strings.Contains(err.Error(), "limited to 8") {
		t.Fatalf("budget error = %v", err)
	}
}

type recordingSearcher struct {
	mu        sync.Mutex
	requests  []Request
	responses []Response
	errors    []error
}

func (*recordingSearcher) Name() string { return "recording" }

func (s *recordingSearcher) Search(_ context.Context, request Request) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := len(s.requests)
	s.requests = append(s.requests, request)
	var response Response
	if index < len(s.responses) {
		response = s.responses[index]
	}
	var err error
	if index < len(s.errors) {
		err = s.errors[index]
	}
	return response, err
}

func (s *recordingSearcher) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

type searcherFunc func(context.Context, Request) (Response, error)

func (searcherFunc) Name() string { return "function" }
func (f searcherFunc) Search(ctx context.Context, request Request) (Response, error) {
	return f(ctx, request)
}
