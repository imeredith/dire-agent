package chatui

import (
	"fmt"
	"strings"

	"github.com/imeredith/dire-agent/agent"
)

func usagePresent(usage agent.Usage) bool {
	return usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CacheReadTokens != 0 ||
		usage.CacheWriteTokens != 0 || usage.TotalTokens != 0 || usage.ContextTokens != 0 || usage.ContextWindow != 0
}

func accumulateUsage(total, current agent.Usage) agent.Usage {
	total.InputTokens += current.InputTokens
	total.OutputTokens += current.OutputTokens
	total.CacheReadTokens += current.CacheReadTokens
	total.CacheWriteTokens += current.CacheWriteTokens
	currentTotal := current.TotalTokens
	if currentTotal == 0 {
		currentTotal = current.InputTokens + current.OutputTokens
	}
	total.TotalTokens += currentTotal
	contextTokens := current.ContextTokens
	if contextTokens == 0 {
		contextTokens = current.InputTokens + current.OutputTokens
	}
	if contextTokens != 0 {
		total.ContextTokens = contextTokens
	}
	if current.ContextWindow != 0 {
		total.ContextWindow = current.ContextWindow
	}
	return total
}

func usageSummary(usage agent.Usage) string {
	context := formatTokens(usage.ContextTokens)
	if usage.ContextWindow > 0 {
		percent := 100 * float64(usage.ContextTokens) / float64(usage.ContextWindow)
		context = fmt.Sprintf("%s/%s (%.1f%%)", context, formatTokens(usage.ContextWindow), percent)
	} else {
		context += " used"
	}
	return fmt.Sprintf("tokens in %s  out %s  cache read %s  write %s  context %s",
		formatTokens(usage.InputTokens), formatTokens(usage.OutputTokens),
		formatTokens(usage.CacheReadTokens), formatTokens(usage.CacheWriteTokens), context)
}

func formatTokens(value int64) string {
	abs := value
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1_000_000:
		number := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(value)/1_000_000), "0"), ".")
		return number + "m"
	case abs >= 1_000:
		number := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", float64(value)/1_000), "0"), ".")
		return number + "k"
	default:
		return fmt.Sprintf("%d", value)
	}
}
