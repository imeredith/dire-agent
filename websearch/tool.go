package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
)

const (
	defaultResults = 5
	maximumResults = 20
	maximumQueries = 8
	maximumDomains = 200
	maximumOutput  = 1 << 20
	maximumQuery   = 10_000
	searchesPerRun = 8
	queryTimeout   = 60 * time.Second
)

var validRecencyFilters = map[string]bool{
	"": true, "day": true, "week": true, "month": true, "year": true,
}

var validDomain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$`)

type tool struct {
	searcher  Searcher
	remaining atomic.Int32
}

type toolInput struct {
	Query         *string   `json:"query"`
	Queries       *[]string `json:"queries"`
	NumResults    int       `json:"numResults"`
	RecencyFilter string    `json:"recencyFilter"`
	DomainFilter  []string  `json:"domainFilter"`
}

// NewTool exposes a Searcher to an agentic loop. Each query invokes a fresh
// provider search session; batch queries deliberately run one at a time so
// provider rate limits and partial failures remain predictable.
func NewTool(searcher Searcher) (agentloop.Tool, error) {
	if searcher == nil {
		return nil, errors.New("websearch: searcher is nil")
	}
	if strings.TrimSpace(searcher.Name()) == "" {
		return nil, errors.New("websearch: searcher name is empty")
	}
	result := &tool{searcher: searcher}
	result.remaining.Store(searchesPerRun)
	return result, nil
}

func (t *tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        Name,
		Description: "Search the live web using an isolated research sub-agent. Returns a concise synthesized answer with clickable source citations. Use query for one search or queries for a batch.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":10000},"queries":{"type":"array","items":{"type":"string","minLength":1,"maxLength":10000},"minItems":1,"maxItems":8},"numResults":{"type":"integer","minimum":1,"maximum":20,"default":5},"recencyFilter":{"type":"string","enum":["day","week","month","year"]},"domainFilter":{"type":"array","items":{"type":"string","minLength":1},"maxItems":200,"description":"Limit results to domains; prefix a domain with - to exclude it. At most 100 allowed and 100 excluded domains."}},"oneOf":[{"required":["query"]},{"required":["queries"]}],"additionalProperties":false}`),
	}
}

func (t *tool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	requests, err := decodeRequests(raw)
	if err != nil {
		return "", err
	}
	if !t.reserve(len(requests)) {
		return "", fmt.Errorf("websearch: this agent run is limited to %d search queries", searchesPerRun)
	}

	sections := make([]string, 0, len(requests))
	failed := 0
	var lastFailure error
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return truncateOutput(strings.Join(sections, "\n\n")), err
		}
		searchContext, cancel := context.WithTimeout(ctx, queryTimeout)
		response, searchErr := t.searcher.Search(searchContext, request)
		cancel()
		if searchErr != nil {
			if ctx.Err() != nil {
				return truncateOutput(strings.Join(sections, "\n\n")), ctx.Err()
			}
			failed++
			lastFailure = searchErr
			sections = append(sections, formatFailure(request.Query, searchErr, len(requests) > 1))
			continue
		}
		if response.Query == "" {
			response.Query = request.Query
		}
		sections = append(sections, formatResponse(response, len(requests) > 1))
	}

	output := truncateOutput(strings.Join(sections, "\n\n"))
	if failed == len(requests) {
		if len(requests) == 1 {
			return "", fmt.Errorf("websearch: search failed for %q: %w", requests[0].Query, lastFailure)
		}
		return output, fmt.Errorf("websearch: all %d searches failed", failed)
	}
	return output, nil
}

func (t *tool) reserve(count int) bool {
	for {
		remaining := t.remaining.Load()
		if int32(count) > remaining {
			return false
		}
		if t.remaining.CompareAndSwap(remaining, remaining-int32(count)) {
			return true
		}
	}
}

func decodeRequests(raw json.RawMessage) ([]Request, error) {
	var input toolInput
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("websearch: invalid arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("websearch: invalid arguments: multiple JSON values")
	}
	hasQuery := input.Query != nil
	hasQueries := input.Queries != nil
	if hasQuery == hasQueries {
		return nil, errors.New("websearch: provide exactly one of query or queries")
	}

	var queries []string
	if hasQuery {
		queries = []string{strings.TrimSpace(*input.Query)}
	} else {
		queries = append([]string(nil), (*input.Queries)...)
	}
	if len(queries) == 0 || len(queries) > maximumQueries {
		return nil, fmt.Errorf("websearch: queries must contain between 1 and %d items", maximumQueries)
	}
	seenQueries := make(map[string]bool, len(queries))
	for index := range queries {
		queries[index] = strings.TrimSpace(queries[index])
		if queries[index] == "" {
			return nil, errors.New("websearch: queries must not contain empty values")
		}
		if len(queries[index]) > maximumQuery {
			return nil, fmt.Errorf("websearch: query must not exceed %d bytes", maximumQuery)
		}
		if seenQueries[queries[index]] {
			return nil, fmt.Errorf("websearch: duplicate query %q", queries[index])
		}
		seenQueries[queries[index]] = true
	}

	if input.NumResults == 0 {
		input.NumResults = defaultResults
	}
	if input.NumResults < 1 || input.NumResults > maximumResults {
		return nil, fmt.Errorf("websearch: numResults must be between 1 and %d", maximumResults)
	}
	input.RecencyFilter = strings.ToLower(strings.TrimSpace(input.RecencyFilter))
	if !validRecencyFilters[input.RecencyFilter] {
		return nil, errors.New("websearch: recencyFilter must be day, week, month, or year")
	}
	allowed, blocked, err := normalizeDomains(input.DomainFilter)
	if err != nil {
		return nil, err
	}

	requests := make([]Request, 0, len(queries))
	for _, query := range queries {
		requests = append(requests, Request{
			Query: query, NumResults: input.NumResults,
			RecencyFilter:  input.RecencyFilter,
			AllowedDomains: append([]string(nil), allowed...),
			BlockedDomains: append([]string(nil), blocked...),
		})
	}
	return requests, nil
}

func normalizeDomains(values []string) ([]string, []string, error) {
	if len(values) > maximumDomains {
		return nil, nil, fmt.Errorf("websearch: domainFilter must contain at most %d items", maximumDomains)
	}
	allowedSet := make(map[string]bool)
	blockedSet := make(map[string]bool)
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		blocked := strings.HasPrefix(raw, "-")
		if blocked {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "-"))
		}
		if raw == "" {
			return nil, nil, errors.New("websearch: domainFilter must not contain empty values")
		}
		candidate := raw
		if !strings.Contains(candidate, "://") {
			candidate = "https://" + candidate
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Port() != "" {
			return nil, nil, fmt.Errorf("websearch: invalid domain %q", raw)
		}
		domain := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
		if !validDomain.MatchString(domain) || strings.Contains(domain, "..") || !validDomainLabels(domain) {
			return nil, nil, fmt.Errorf("websearch: invalid domain %q", raw)
		}
		if blocked {
			blockedSet[domain] = true
		} else {
			allowedSet[domain] = true
		}
	}
	allowed := mapKeys(allowedSet)
	blocked := mapKeys(blockedSet)
	if len(allowed) > 100 || len(blocked) > 100 {
		return nil, nil, errors.New("websearch: domainFilter accepts at most 100 allowed and 100 excluded domains")
	}
	for _, domain := range allowed {
		if blockedSet[domain] {
			return nil, nil, fmt.Errorf("websearch: domain %q cannot be both allowed and blocked", domain)
		}
	}
	return allowed, blocked, nil
}

func validDomainLabels(domain string) bool {
	if len(domain) > 253 {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func formatResponse(response Response, includeHeading bool) string {
	var output strings.Builder
	if includeHeading {
		fmt.Fprintf(&output, "## %s\n\n", response.Query)
	}
	answer := strings.TrimSpace(response.Answer)
	if answer != "" {
		output.WriteString(answer)
	}
	validCitations := make([]Citation, 0, len(response.Citations))
	for _, citation := range response.Citations {
		if citation.URL = safeCitationURL(citation.URL); citation.URL != "" {
			validCitations = append(validCitations, citation)
		}
	}
	if len(validCitations) != 0 {
		if answer != "" {
			output.WriteString("\n\n")
		}
		output.WriteString("Sources:\n")
		for _, citation := range validCitations {
			title := strings.TrimSpace(strings.ReplaceAll(citation.Title, "\n", " "))
			if title == "" {
				title = citation.URL
			}
			title = strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]").Replace(title)
			safeURL := strings.NewReplacer("(", "%28", ")", "%29").Replace(citation.URL)
			fmt.Fprintf(&output, "- [%s](%s)", title, safeURL)
			if snippet := strings.TrimSpace(strings.ReplaceAll(citation.Snippet, "\n", " ")); snippet != "" {
				fmt.Fprintf(&output, " — %s", snippet)
			}
			output.WriteByte('\n')
		}
	}
	if output.Len() == 0 {
		output.WriteString("No search answer or citations were returned.")
	}
	return strings.TrimSpace(output.String())
}

func safeCitationURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func formatFailure(query string, err error, includeHeading bool) string {
	message := "Search failed: " + err.Error()
	if includeHeading {
		return fmt.Sprintf("## %s\n\n%s", query, message)
	}
	return message
}

func truncateOutput(output string) string {
	output = strings.ToValidUTF8(output, "�")
	if len(output) <= maximumOutput {
		return output
	}
	cut := maximumOutput
	for cut > 0 && !utf8.RuneStart(output[cut]) {
		cut--
	}
	return output[:cut] + "\n\n[web search output truncated]"
}
