package openrouter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/dire-kiwi/dire-agent/agent"
)

type pendingToolItem struct {
	fields    map[string]json.RawMessage
	arguments strings.Builder
}

func readResponseStream(ctx context.Context, reader io.Reader, onEvent func(agent.ModelEvent)) (streamResult, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var result streamResult
	var dataLines []string
	completed := false
	reasoningDone := false
	var reasoning strings.Builder
	pending := make(map[int]*pendingToolItem)

	emitReasoningDone := func(text string) {
		if reasoningDone {
			return
		}
		text = cleanReasoningSummary(text)
		if strings.TrimSpace(text) == "" {
			return
		}
		reasoningDone = true
		if onEvent != nil {
			onEvent(agent.ModelEvent{Type: "reasoning_done", Text: text})
		}
	}
	appendItem := func(raw json.RawMessage, emit bool) {
		if len(raw) == 0 || string(raw) == "null" || containsItem(result.items, raw) {
			return
		}
		copyOfItem := append(json.RawMessage(nil), raw...)
		result.items = append(result.items, copyOfItem)
		if emit && onEvent != nil {
			onEvent(agent.ModelEvent{Type: "item_done", Item: append(json.RawMessage(nil), raw...)})
		}
		if !reasoningDone {
			emitReasoningDone(reasoningSummaryFromItem(raw))
		}
	}
	flushPending := func() {
		for index, item := range pending {
			if item.arguments.Len() != 0 {
				item.fields["arguments"], _ = json.Marshal(item.arguments.String())
			}
			if encoded, err := json.Marshal(item.fields); err == nil {
				appendItem(encoded, true)
			}
			delete(pending, index)
		}
	}

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
			return fmt.Errorf("openrouter: decode response stream event: %w", err)
		}
		if result.responseID == "" {
			result.responseID = firstNonEmpty(event.ResponseID, event.Response.ID)
		}
		switch event.Type {
		case "response.output_text.delta", "response.content_part.delta":
			delta := eventDeltaText(event.Delta)
			result.deltas.WriteString(delta)
			if onEvent != nil && delta != "" {
				onEvent(agent.ModelEvent{Type: "text_delta", Delta: delta})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning.delta":
			delta := eventDeltaText(event.Delta)
			reasoning.WriteString(delta)
			if onEvent != nil && delta != "" {
				onEvent(agent.ModelEvent{Type: "reasoning_delta", Delta: delta})
			}
		case "response.reasoning_summary_text.done", "response.reasoning.done":
			text := firstNonEmpty(event.Text, eventDeltaText(event.Delta), reasoning.String())
			emitReasoningDone(text)
		case "response.output_item.added":
			if item := newPendingToolItem(event.Item); item != nil {
				pending[event.OutputIndex] = item
			}
		case "response.function_call_arguments.delta":
			if item := findPendingToolItem(pending, event.OutputIndex, event.ItemID); item != nil {
				item.arguments.WriteString(eventDeltaText(event.Delta))
			}
		case "response.function_call_arguments.done":
			if item := findPendingToolItem(pending, event.OutputIndex, event.ItemID); item != nil {
				if event.Arguments != "" {
					item.arguments.Reset()
					item.arguments.WriteString(event.Arguments)
				}
			}
		case "response.output_item.done":
			appendItem(event.Item, true)
			delete(pending, event.OutputIndex)
		case "response.completed", "response.done":
			if event.Response.Error != nil {
				return streamAPIError(event.Response.Error, firstNonEmpty(event.Response.ErrorType, event.ErrorType), "response failed")
			}
			completed = true
			if event.Response.ID != "" {
				result.responseID = event.Response.ID
			}
			contextWindow := event.Response.ContextWindow
			if contextWindow == 0 {
				contextWindow = event.Response.Usage.ContextWindow
			}
			result.usage = event.Response.Usage.agentUsage(contextWindow)
			for _, item := range event.Response.Output {
				appendItem(item, false)
			}
			// A completed response's output is authoritative. Only synthesize
			// pending streamed calls that the terminal payload omitted.
			flushPending()
		case "response.failed":
			return streamAPIError(event.Response.Error, firstNonEmpty(event.Response.ErrorType, event.ErrorType), "response failed")
		case "response.incomplete":
			reason := "unknown"
			if event.Response.IncompleteDetails != nil && event.Response.IncompleteDetails.Reason != "" {
				reason = event.Response.IncompleteDetails.Reason
			}
			return &APIError{Code: "response_incomplete", Message: "incomplete response: " + reason}
		case "error":
			return streamAPIError(event.Error, event.ErrorType, "stream error")
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
				emitReasoningDone(reasoning.String())
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
		return streamResult{}, fmt.Errorf("openrouter: read response stream: %w", err)
	}
	if err := process(); err != nil {
		return streamResult{}, err
	}
	if !completed {
		return streamResult{}, errors.New("openrouter: response stream closed before response.done")
	}
	emitReasoningDone(reasoning.String())
	return result, nil
}

func eventDeltaText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var value struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if json.Unmarshal(raw, &value) == nil {
		return firstNonEmpty(value.Text, value.Content)
	}
	return ""
}

func newPendingToolItem(raw json.RawMessage) *pendingToolItem {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || rawScalarString(fields["type"]) != "function_call" {
		return nil
	}
	item := &pendingToolItem{fields: fields}
	if arguments := normalizeArguments(fields["arguments"]); string(arguments) != "{}" {
		item.arguments.Write(arguments)
	}
	return item
}

func findPendingToolItem(pending map[int]*pendingToolItem, outputIndex int, itemID string) *pendingToolItem {
	if item := pending[outputIndex]; item != nil {
		return item
	}
	if itemID == "" {
		return nil
	}
	for _, item := range pending {
		if rawScalarString(item.fields["id"]) == itemID {
			return item
		}
	}
	return nil
}

func containsItem(items []json.RawMessage, candidate json.RawMessage) bool {
	wantKey := responseItemKey(candidate)
	for _, item := range items {
		if wantKey != "" && responseItemKey(item) == wantKey {
			return true
		}
		if string(item) == string(candidate) {
			return true
		}
	}
	return false
}

func responseItemKey(raw json.RawMessage) string {
	var item struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		CallID string `json:"call_id"`
	}
	if json.Unmarshal(raw, &item) != nil {
		return ""
	}
	if item.ID != "" {
		return item.Type + ":id:" + item.ID
	}
	if item.CallID != "" {
		return item.Type + ":call:" + item.CallID
	}
	return ""
}

func reasoningSummaryFromItem(raw json.RawMessage) string {
	var item struct {
		Type    string            `json:"type"`
		Summary []json.RawMessage `json:"summary"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Type != "reasoning" {
		return ""
	}
	parts := make([]string, 0, len(item.Summary))
	for _, rawPart := range item.Summary {
		var text string
		if json.Unmarshal(rawPart, &text) != nil {
			var part struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(rawPart, &part) == nil {
				text = part.Text
			}
		}
		if text = strings.TrimSpace(text); text != "" {
			parts = append(parts, text)
		}
	}
	return cleanReasoningSummary(strings.Join(parts, "\n\n"))
}

func streamAPIError(streamError *streamError, errorType, fallback string) error {
	if streamError == nil {
		return &APIError{ErrorType: errorType, Message: fallback}
	}
	code := firstNonEmpty(rawScalarString(streamError.Code), streamError.Type)
	errorType = firstNonEmpty(streamError.ErrorType, streamError.Metadata.ErrorType, errorType)
	message := firstNonEmpty(streamError.Message, fallback)
	return &APIError{Code: code, ErrorType: errorType, Message: message}
}

var reasoningHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func cleanReasoningSummary(text string) string {
	return strings.TrimSpace(reasoningHTMLComment.ReplaceAllString(text, ""))
}
