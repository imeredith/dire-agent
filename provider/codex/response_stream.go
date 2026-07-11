package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/imeredith/dire-agent/agent"
)

func readResponseStream(ctx context.Context, reader io.Reader, onEvent func(agent.ModelEvent)) (streamResult, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var result streamResult
	var dataLines []string
	completed := false
	reasoningDone := false
	var reasoning strings.Builder

	process := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if data == "[DONE]" {
			return nil
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("codex: decode response stream event: %w", err)
		}
		switch event.Type {
		case "response.output_text.delta":
			result.deltas.WriteString(event.Delta)
			if onEvent != nil {
				onEvent(agent.ModelEvent{Type: "text_delta", Delta: event.Delta})
			}
		case "response.reasoning_summary_text.delta":
			reasoning.WriteString(event.Delta)
			if onEvent != nil {
				onEvent(agent.ModelEvent{Type: "reasoning_delta", Delta: event.Delta})
			}
		case "response.reasoning_summary_text.done":
			text := event.Text
			if text == "" {
				text = reasoning.String()
			}
			text = cleanReasoningSummary(text)
			reasoningDone = true
			if onEvent != nil && strings.TrimSpace(text) != "" {
				onEvent(agent.ModelEvent{Type: "reasoning_done", Text: text})
			}
		case "response.output_item.done":
			if len(event.Item) != 0 && string(event.Item) != "null" {
				result.items = append(result.items, append(json.RawMessage(nil), event.Item...))
				if onEvent != nil {
					onEvent(agent.ModelEvent{Type: "item_done", Item: append(json.RawMessage(nil), event.Item...)})
				}
				if !reasoningDone {
					if text := reasoningSummaryFromItem(event.Item); strings.TrimSpace(text) != "" {
						reasoningDone = true
						if onEvent != nil {
							onEvent(agent.ModelEvent{Type: "reasoning_done", Text: text})
						}
					}
				}
			}
		case "response.completed":
			completed = true
			result.responseID = event.Response.ID
			contextWindow := event.Response.ContextWindow
			if contextWindow == 0 {
				contextWindow = event.Response.Usage.ContextWindow
			}
			if contextWindow == 0 {
				contextWindow = contextWindowForModel(event.Response.Model)
			}
			result.usage = event.Response.Usage.agentUsage(contextWindow)
			if len(result.items) == 0 {
				result.items = append(result.items, event.Response.Output...)
			}
		case "response.failed":
			return streamAPIError(event.Response.Error, "response failed")
		case "response.incomplete":
			reason := "unknown"
			if event.Response.IncompleteDetails != nil && event.Response.IncompleteDetails.Reason != "" {
				reason = event.Response.IncompleteDetails.Reason
			}
			return &APIError{Code: "response_incomplete", Message: "incomplete response: " + reason}
		case "error":
			return streamAPIError(event.Error, "stream error")
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := process(); err != nil {
				return streamResult{}, err
			}
			if completed {
				if !reasoningDone && onEvent != nil && strings.TrimSpace(reasoning.String()) != "" {
					onEvent(agent.ModelEvent{Type: "reasoning_done", Text: cleanReasoningSummary(reasoning.String())})
				}
				return result, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if ctx.Err() != nil {
		return streamResult{}, ctx.Err()
	}
	if err := scanner.Err(); err != nil {
		return streamResult{}, fmt.Errorf("codex: read response stream: %w", err)
	}
	if err := process(); err != nil {
		return streamResult{}, err
	}
	if !completed {
		return streamResult{}, errors.New("codex: response stream closed before response.completed")
	}
	if !reasoningDone && onEvent != nil && strings.TrimSpace(reasoning.String()) != "" {
		onEvent(agent.ModelEvent{Type: "reasoning_done", Text: cleanReasoningSummary(reasoning.String())})
	}
	return result, nil
}

func reasoningSummaryFromItem(raw json.RawMessage) string {
	var item struct {
		Type    string `json:"type"`
		Summary []struct {
			Text string `json:"text"`
		} `json:"summary"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Type != "reasoning" {
		return ""
	}
	parts := make([]string, 0, len(item.Summary))
	for _, part := range item.Summary {
		if text := strings.TrimSpace(part.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return cleanReasoningSummary(strings.Join(parts, "\n\n"))
}

var reasoningHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func cleanReasoningSummary(text string) string {
	return strings.TrimSpace(reasoningHTMLComment.ReplaceAllString(text, ""))
}
