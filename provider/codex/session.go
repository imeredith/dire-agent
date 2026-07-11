package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/imeredith/dire-agent/agent"
)

type session struct {
	provider     *Provider
	id           string
	model        string
	instructions string
	mu           sync.Mutex
	history      []json.RawMessage
}

func (s *session) ID() string { return s.id }

func (s *session) Run(ctx context.Context, prompt string) (agent.Result, error) {
	if strings.TrimSpace(prompt) == "" {
		return agent.Result{}, errors.New("codex: prompt is empty")
	}
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	if err != nil {
		return agent.Result{}, err
	}
	if len(step.ToolCalls) != 0 {
		return agent.Result{}, errors.New("codex: model requested tools but Run was called without an agentic tool loop")
	}
	return step.Result, nil
}

func (s *session) Step(ctx context.Context, step agent.StepRequest) (agent.StepResult, error) {
	if len(step.UserMessages) == 0 && len(step.Images) == 0 && len(step.ToolResults) == 0 {
		return agent.StepResult{}, errors.New("codex: model step has no new input")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var additions []json.RawMessage
	for _, result := range step.ToolResults {
		output := result.Output
		if result.IsError {
			output = "Tool error: " + output
		}
		item, err := json.Marshal(map[string]any{
			"type": "function_call_output", "call_id": result.CallID, "output": output,
		})
		if err != nil {
			return agent.StepResult{}, fmt.Errorf("codex: encode tool result: %w", err)
		}
		additions = append(additions, item)
	}
	for index, message := range step.UserMessages {
		if strings.TrimSpace(message) == "" {
			continue
		}
		content := []map[string]any{{"type": "input_text", "text": message}}
		if index == 0 {
			content = append(content, responseImageContent(step.Images)...)
		}
		item, err := json.Marshal(map[string]any{
			"type": "message", "role": "user",
			"content": content,
		})
		if err != nil {
			return agent.StepResult{}, fmt.Errorf("codex: encode user input: %w", err)
		}
		additions = append(additions, item)
	}
	if len(step.UserMessages) == 0 && len(step.Images) != 0 {
		item, err := json.Marshal(map[string]any{
			"type": "message", "role": "user", "content": responseImageContent(step.Images),
		})
		if err != nil {
			return agent.StepResult{}, fmt.Errorf("codex: encode image input: %w", err)
		}
		additions = append(additions, item)
	}
	input := append([]json.RawMessage(nil), s.history...)
	input = append(input, additions...)

	tools := make([]responseToolDefinition, 0, len(step.Tools))
	for _, tool := range step.Tools {
		tools = append(tools, responseToolDefinition{
			Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters,
		})
	}
	reasoningEffort := strings.TrimSpace(step.ReasoningEffort)
	if reasoningEffort == "" {
		reasoningEffort = "medium"
	}

	responsesLite := usesResponsesLite(s.model)
	wireInput, wireTools, wireInstructions := input, tools, s.instructions
	reasoning := map[string]string{"effort": reasoningEffort, "summary": "auto"}
	if responsesLite {
		var err error
		wireInput, err = responsesLiteInput(s.instructions, tools, input)
		if err != nil {
			return agent.StepResult{}, err
		}
		wireTools, wireInstructions = nil, ""
		reasoning["context"] = "all_turns"
	}

	request := responsesRequest{
		Model: codexSubscriptionModel(s.model), Instructions: wireInstructions,
		Input: wireInput, Tools: wireTools, ToolChoice: "auto", ParallelToolCalls: false,
		Reasoning: reasoning,
		Store:     false, Stream: true, Include: []string{"reasoning.encrypted_content"},
		PromptCacheKey: s.id,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return agent.StepResult{}, fmt.Errorf("codex: encode response request: %w", err)
	}

	response, err := s.provider.send(ctx, s.id, payload, responsesLite)
	if err != nil {
		return agent.StepResult{}, err
	}
	defer response.Body.Close()

	streamed, err := readResponseStream(ctx, response.Body, step.OnEvent)
	if err != nil {
		return agent.StepResult{}, err
	}
	if streamed.usage.ContextWindow == 0 {
		streamed.usage.ContextWindow = contextWindowForModel(s.model)
	}
	s.history = append(input, streamed.historyItems()...)
	turnID := streamed.responseID
	if turnID == "" {
		turnID, _ = randomID()
	}
	return agent.StepResult{
		Result: agent.Result{
			Text: streamed.finalText(), Provider: providerName, SessionID: s.id,
			TurnID: turnID, Usage: streamed.usage,
		},
		ToolCalls: streamed.toolCalls(),
	}, nil
}

func responseImageContent(images []agent.ImageInput) []map[string]any {
	content := make([]map[string]any, 0, len(images))
	for _, image := range images {
		if len(image.Data) == 0 || strings.TrimSpace(image.MimeType) == "" {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(image.Data)
		content = append(content, map[string]any{
			"type": "input_image", "image_url": "data:" + image.MimeType + ";base64," + encoded,
			"detail": "auto",
		})
	}
	return content
}

func usesResponsesLite(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"openai.", "openai/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	return strings.HasPrefix(model, "gpt-5.6")
}

func responsesLiteInput(instructions string, tools []responseToolDefinition, input []json.RawMessage) ([]json.RawMessage, error) {
	prefix := make([]json.RawMessage, 0, 2)
	additionalTools, err := json.Marshal(map[string]any{
		"type": "additional_tools", "role": "developer", "tools": tools,
	})
	if err != nil {
		return nil, fmt.Errorf("codex: encode Responses Lite tools: %w", err)
	}
	prefix = append(prefix, additionalTools)
	if instructions != "" {
		developerMessage, err := json.Marshal(map[string]any{
			"type": "message", "role": "developer",
			"content": []map[string]string{{"type": "input_text", "text": instructions}},
		})
		if err != nil {
			return nil, fmt.Errorf("codex: encode Responses Lite instructions: %w", err)
		}
		prefix = append(prefix, developerMessage)
	}
	return append(prefix, input...), nil
}

func (s *session) State() (agent.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.history)
	if err != nil {
		return agent.SessionState{}, fmt.Errorf("codex: encode session state: %w", err)
	}
	return agent.SessionState{ID: s.id, Provider: providerName, Data: data}, nil
}
