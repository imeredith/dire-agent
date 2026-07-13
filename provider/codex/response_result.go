package codex

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/dire-kiwi/dire-agent/agent"
)

func (r *streamResult) toolCalls() []agent.ToolCall {
	var calls []agent.ToolCall
	for _, raw := range r.items {
		var item struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if json.Unmarshal(raw, &item) != nil || item.Type != "function_call" || item.Name == "" {
			continue
		}
		arguments := json.RawMessage(item.Arguments)
		if !json.Valid(arguments) {
			arguments = json.RawMessage(`{}`)
		}
		calls = append(calls, agent.ToolCall{ID: item.CallID, Name: item.Name, Arguments: arguments})
	}
	return calls
}

func streamAPIError(streamError *streamError, fallback string) error {
	if streamError == nil {
		return &APIError{Message: fallback}
	}
	code := streamError.Code
	if code == "" {
		code = streamError.Type
	}
	message := streamError.Message
	if message == "" {
		message = fallback
	}
	return &APIError{Code: code, Message: message}
}

func (r *streamResult) finalText() string {
	var finalAnswers []string
	var unclassified []string
	for _, raw := range r.items {
		var item struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Phase   string `json:"phase"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(raw, &item) != nil || item.Type != "message" || item.Role != "assistant" {
			continue
		}
		var text strings.Builder
		for _, content := range item.Content {
			if content.Type == "output_text" {
				text.WriteString(content.Text)
			}
		}
		if text.Len() == 0 {
			continue
		}
		switch item.Phase {
		case "final_answer":
			finalAnswers = append(finalAnswers, text.String())
		case "":
			unclassified = append(unclassified, text.String())
		}
	}
	if len(finalAnswers) != 0 {
		return strings.Join(finalAnswers, "\n")
	}
	if len(unclassified) != 0 {
		return strings.Join(unclassified, "\n")
	}
	return r.deltas.String()
}

func (r *streamResult) historyItems() []json.RawMessage {
	items := make([]json.RawMessage, 0, len(r.items))
	for _, raw := range r.items {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		delete(item, "id")
		encoded, err := json.Marshal(item)
		if err == nil {
			items = append(items, encoded)
		}
	}
	return items
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "dire_agent_" + hex.EncodeToString(value[:]), nil
}
