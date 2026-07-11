package codex

import (
	"encoding/json"
	"strings"

	"github.com/imeredith/dire-agent/agent"
)

type responsesRequest struct {
	Model             string                   `json:"model"`
	Instructions      string                   `json:"instructions,omitempty"`
	Input             []json.RawMessage        `json:"input"`
	Tools             []responseToolDefinition `json:"tools,omitempty"`
	ToolChoice        string                   `json:"tool_choice"`
	ParallelToolCalls bool                     `json:"parallel_tool_calls"`
	Reasoning         map[string]string        `json:"reasoning,omitempty"`
	Store             bool                     `json:"store"`
	Stream            bool                     `json:"stream"`
	Include           []string                 `json:"include"`
	PromptCacheKey    string                   `json:"prompt_cache_key,omitempty"`
}

type responseToolDefinition struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type streamResult struct {
	responseID string
	deltas     strings.Builder
	items      []json.RawMessage
	usage      agent.Usage
}

type responsesStreamEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Text     string          `json:"text"`
	Item     json.RawMessage `json:"item"`
	Response struct {
		ID                string            `json:"id"`
		Model             string            `json:"model"`
		ContextWindow     int64             `json:"context_window"`
		Output            []json.RawMessage `json:"output"`
		Usage             responsesUsage    `json:"usage"`
		Error             *streamError      `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
	Error *streamError `json:"error"`
}

type streamError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// responsesUsage includes the public Responses API fields plus cache aliases
// emitted by earlier Codex/Anthropic-compatible backends. The aliases are
// alternatives, not additive counters.
type responsesUsage struct {
	InputTokens              int64              `json:"input_tokens"`
	OutputTokens             int64              `json:"output_tokens"`
	TotalTokens              int64              `json:"total_tokens"`
	ContextWindow            int64              `json:"context_window"`
	CachedTokens             int64              `json:"cached_tokens"`
	CachedInputTokens        int64              `json:"cached_input_tokens"`
	CacheReadTokens          int64              `json:"cache_read_tokens"`
	CacheReadInputTokens     int64              `json:"cache_read_input_tokens"`
	CacheWriteTokens         int64              `json:"cache_write_tokens"`
	CacheCreationTokens      int64              `json:"cache_creation_tokens"`
	CacheCreationInputTokens int64              `json:"cache_creation_input_tokens"`
	CacheCreation            cacheCreationUsage `json:"cache_creation"`
	InputTokensDetails       usageTokenDetails  `json:"input_tokens_details"`
}

type usageTokenDetails struct {
	CachedTokens             int64              `json:"cached_tokens"`
	CachedInputTokens        int64              `json:"cached_input_tokens"`
	CacheReadTokens          int64              `json:"cache_read_tokens"`
	CacheReadInputTokens     int64              `json:"cache_read_input_tokens"`
	CacheWriteTokens         int64              `json:"cache_write_tokens"`
	CacheCreationTokens      int64              `json:"cache_creation_tokens"`
	CacheCreationInputTokens int64              `json:"cache_creation_input_tokens"`
	CacheCreation            cacheCreationUsage `json:"cache_creation"`
}

type cacheCreationUsage struct {
	Value                  int64 `json:"-"`
	Tokens                 int64 `json:"tokens"`
	InputTokens            int64 `json:"input_tokens"`
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
}

func (u *cacheCreationUsage) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err == nil {
		u.Value = value
		return nil
	}
	type plain cacheCreationUsage
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = cacheCreationUsage(decoded)
	return nil
}
