// Package websearch defines the provider-neutral web search capability.
package websearch

import (
	"context"

	"github.com/dire-kiwi/dire-agent/agent"
)

const Name = "web_search"

// Searcher runs a one-shot, isolated search agent. Provider implementations
// should not add the search turn to an ordinary conversation's history.
type Searcher interface {
	Name() string
	Search(context.Context, Request) (Response, error)
}

// Request is the portable subset of hosted web-search controls.
type Request struct {
	Query          string
	NumResults     int
	RecencyFilter  string
	AllowedDomains []string
	BlockedDomains []string
}

// Citation identifies a web source used by a search response.
type Citation struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// Response is the synthesized result returned by an isolated search agent.
type Response struct {
	Query     string      `json:"query,omitempty"`
	Answer    string      `json:"answer,omitempty"`
	Citations []Citation  `json:"citations,omitempty"`
	Provider  string      `json:"provider,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
	TurnID    string      `json:"turn_id,omitempty"`
	Usage     agent.Usage `json:"usage,omitempty"`
}
