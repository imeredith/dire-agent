package codex

import (
	"strings"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/modelcatalog"
)

func (u responsesUsage) agentUsage(contextWindow int64) agent.Usage {
	cacheRead := firstNonZero(
		u.InputTokensDetails.CachedTokens, u.CachedTokens,
		u.InputTokensDetails.CachedInputTokens, u.CachedInputTokens,
		u.InputTokensDetails.CacheReadTokens, u.CacheReadTokens,
		u.InputTokensDetails.CacheReadInputTokens, u.CacheReadInputTokens,
	)
	cacheWrite := firstNonZero(
		u.InputTokensDetails.CacheWriteTokens, u.CacheWriteTokens,
		u.InputTokensDetails.CacheCreationInputTokens, u.CacheCreationInputTokens,
		u.InputTokensDetails.CacheCreationTokens, u.CacheCreationTokens,
		u.InputTokensDetails.CacheCreation.total(), u.CacheCreation.total(),
	)
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens
	}
	return agent.Usage{
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite, TotalTokens: total,
		ContextTokens: u.InputTokens + u.OutputTokens, ContextWindow: contextWindow,
	}
}

func (u cacheCreationUsage) total() int64 {
	if total := u.Ephemeral5mInputTokens + u.Ephemeral1hInputTokens; total != 0 {
		return total
	}
	return firstNonZero(u.Value, u.Tokens, u.InputTokens)
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func contextWindowForModel(model string) int64 {
	return modelcatalog.ContextWindow(model)
}

// The public Responses API accepts gpt-5.6 as an alias for the Sol variant,
// while the ChatGPT-backed Codex subscription endpoint currently requires the
// concrete variant name.
func codexSubscriptionModel(model string) string {
	if strings.EqualFold(strings.TrimSpace(model), "gpt-5.6") {
		return "gpt-5.6-sol"
	}
	return model
}
