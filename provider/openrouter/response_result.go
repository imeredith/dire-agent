package openrouter

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/dire-kiwi/dire-agent/agent"
)

func (r *streamResult) toolCalls() []agent.ToolCall {
	var calls []agent.ToolCall
	seen := make(map[string]bool)
	for _, raw := range r.items {
		var item struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(raw, &item) != nil || item.Type != "function_call" || item.Name == "" {
			continue
		}
		id := firstNonEmpty(item.CallID, item.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		arguments := normalizeArguments(item.Arguments)
		calls = append(calls, agent.ToolCall{ID: id, Name: item.Name, Arguments: arguments})
	}
	return calls
}

func normalizeArguments(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		raw = json.RawMessage(encoded)
	}
	if !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), raw...)
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

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "dire_agent_" + hex.EncodeToString(value[:]), nil
}
