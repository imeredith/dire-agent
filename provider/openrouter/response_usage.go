package openrouter

import (
	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/modelcatalog"
)

func (u responsesUsage) agentUsage(contextWindow int64) agent.Usage {
	input := firstNonZeroInt(u.InputTokens, u.PromptTokens)
	output := firstNonZeroInt(u.OutputTokens, u.CompletionTokens)
	cacheRead := firstNonZeroInt(
		u.InputTokensDetails.CachedTokens, u.CachedTokens,
		u.InputTokensDetails.CachedInputTokens, u.CachedInputTokens,
		u.InputTokensDetails.CacheReadTokens, u.CacheReadTokens,
		u.InputTokensDetails.CacheReadInputTokens, u.CacheReadInputTokens,
	)
	cacheWrite := firstNonZeroInt(
		u.InputTokensDetails.CacheWriteTokens, u.CacheWriteTokens,
		u.InputTokensDetails.CacheCreationInputTokens, u.CacheCreationInputTokens,
		u.InputTokensDetails.CacheCreationTokens, u.CacheCreationTokens,
		u.InputTokensDetails.CacheCreation.total(), u.CacheCreation.total(),
	)
	total := u.TotalTokens
	if total == 0 {
		total = input + output
	}
	if contextWindow == 0 {
		contextWindow = u.ContextWindow
	}
	return agent.Usage{
		InputTokens: input, OutputTokens: output,
		CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite,
		TotalTokens: total, ContextTokens: input + output,
		ContextWindow: contextWindow,
	}
}

func (u cacheCreationUsage) total() int64 {
	if total := u.Ephemeral5mInputTokens + u.Ephemeral1hInputTokens; total != 0 {
		return total
	}
	return firstNonZeroInt(u.Value, u.Tokens, u.InputTokens)
}

func firstNonZeroInt(values ...int64) int64 {
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
