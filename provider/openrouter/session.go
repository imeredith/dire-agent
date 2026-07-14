package openrouter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/dire-kiwi/dire-agent/agent"
)

type session struct {
	provider     *Provider
	id           string
	model        string
	instructions string
	mu           sync.Mutex
	history      []json.RawMessage
}

var (
	_ agent.StepSession     = (*session)(nil)
	_ agent.StatefulSession = (*session)(nil)
)

func (s *session) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *session) Run(ctx context.Context, prompt string) (agent.Result, error) {
	if strings.TrimSpace(prompt) == "" {
		return agent.Result{}, errors.New("openrouter: prompt is empty")
	}
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	if err != nil {
		return agent.Result{}, err
	}
	if len(step.ToolCalls) != 0 {
		return agent.Result{}, errors.New("openrouter: model requested tools but Run was called without an agentic tool loop")
	}
	return step.Result, nil
}

func (s *session) Step(ctx context.Context, step agent.StepRequest) (agent.StepResult, error) {
	if s == nil || s.provider == nil {
		return agent.StepResult{}, errors.New("openrouter: session is not initialized")
	}
	if len(step.UserMessages) == 0 && len(step.Images) == 0 && len(step.ToolResults) == 0 {
		return agent.StepResult{}, errors.New("openrouter: model step has no new input")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	additions, err := responseInput(step)
	if err != nil {
		return agent.StepResult{}, err
	}
	if len(additions) == 0 {
		return agent.StepResult{}, errors.New("openrouter: model step has no usable input")
	}
	input := append(cloneRawMessages(s.history), additions...)

	tools := make([]responseToolDefinition, 0, len(step.Tools))
	for _, tool := range step.Tools {
		tools = append(tools, responseToolDefinition{
			Type: "function", Name: tool.Name, Description: tool.Description,
			Parameters: tool.Parameters,
		})
	}
	var reasoning map[string]string
	if effort := strings.TrimSpace(step.ReasoningEffort); effort != "" {
		if strings.EqualFold(effort, "off") {
			effort = "none"
		}
		reasoning = map[string]string{"effort": effort}
	}
	request := responsesRequest{
		Model: s.model, Instructions: s.instructions, Input: input,
		Tools: tools, Store: false, Stream: true, SessionID: s.id,
		Reasoning: reasoning,
	}
	if len(tools) != 0 {
		request.ToolChoice = "auto"
		request.ParallelToolCalls = false
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return agent.StepResult{}, fmt.Errorf("openrouter: encode response request: %w", err)
	}

	response, err := s.provider.send(ctx, "POST", "/responses", payload, "text/event-stream")
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

	s.history = append(input, cloneRawMessages(streamed.items)...)
	turnID := streamed.responseID
	if turnID == "" {
		turnID, _ = randomID()
	}
	return agent.StepResult{
		Result: agent.Result{
			Text: streamed.finalText(), Provider: providerName,
			SessionID: s.id, TurnID: turnID, Usage: streamed.usage,
		},
		ToolCalls: streamed.toolCalls(),
	}, nil
}

func responseInput(step agent.StepRequest) ([]json.RawMessage, error) {
	var additions []json.RawMessage
	for _, result := range step.ToolResults {
		if strings.TrimSpace(result.CallID) == "" {
			continue
		}
		output := result.Output
		if result.IsError {
			output = "Tool error: " + output
		}
		item, err := json.Marshal(map[string]any{
			"type": "function_call_output", "call_id": result.CallID, "output": output,
		})
		if err != nil {
			return nil, fmt.Errorf("openrouter: encode tool result: %w", err)
		}
		additions = append(additions, item)
	}

	images := responseImageContent(step.Images)
	imagesAdded := false
	for _, message := range step.UserMessages {
		if strings.TrimSpace(message) == "" {
			continue
		}
		content := []map[string]any{{"type": "input_text", "text": message}}
		if !imagesAdded && len(images) != 0 {
			content = append(content, images...)
			imagesAdded = true
		}
		item, err := json.Marshal(map[string]any{
			"type": "message", "role": "user", "content": content,
		})
		if err != nil {
			return nil, fmt.Errorf("openrouter: encode user input: %w", err)
		}
		additions = append(additions, item)
	}
	if !imagesAdded && len(images) != 0 {
		item, err := json.Marshal(map[string]any{
			"type": "message", "role": "user", "content": images,
		})
		if err != nil {
			return nil, fmt.Errorf("openrouter: encode image input: %w", err)
		}
		additions = append(additions, item)
	}
	return additions, nil
}

func responseImageContent(images []agent.ImageInput) []map[string]any {
	content := make([]map[string]any, 0, len(images))
	for _, image := range images {
		mimeType := strings.TrimSpace(image.MimeType)
		if len(image.Data) == 0 || mimeType == "" || strings.ContainsAny(mimeType, "\r\n,") {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(image.Data)
		content = append(content, map[string]any{
			"type": "input_image", "image_url": "data:" + mimeType + ";base64," + encoded,
			"detail": "auto",
		})
	}
	return content
}

func (s *session) State() (agent.SessionState, error) {
	if s == nil {
		return agent.SessionState{}, errors.New("openrouter: session is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.history)
	if err != nil {
		return agent.SessionState{}, fmt.Errorf("openrouter: encode session state: %w", err)
	}
	return agent.SessionState{ID: s.id, Provider: providerName, Data: data}, nil
}

func cloneRawMessages(messages []json.RawMessage) []json.RawMessage {
	cloned := make([]json.RawMessage, len(messages))
	for index, message := range messages {
		cloned[index] = append(json.RawMessage(nil), message...)
	}
	return cloned
}
