// Package agentloop runs model/tool/model cycles on top of an agent.StepSession.
package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/imeredith/dire-agent/agent"
)

// Tool is a model-callable capability.
type Tool interface {
	Definition() agent.ToolDefinition
	Execute(context.Context, json.RawMessage) (string, error)
}

// Event mirrors the useful lifecycle events exposed by Pi's agent session.
type Event struct {
	Type       string          `json:"type"`
	Step       int             `json:"step,omitempty"`
	MessageID  string          `json:"message_id,omitempty"`
	Delta      string          `json:"delta,omitempty"`
	Text       string          `json:"text,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Output     string          `json:"output,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	Usage      *agent.Usage    `json:"usage,omitempty"`
}

// Config controls an agentic run.
type Config struct {
	Session         agent.StepSession
	Tools           map[string]Tool
	ReasoningEffort string
	MaxSteps        int
	Emit            func(Event)
	// TakeSteering returns messages queued while the agent is running. It is
	// called after each model step and its tool executions.
	TakeSteering func() []string
	Hooks        Hooks
}

// Hooks are ordered capability middleware. An error stops the run; a before
// tool hook may deliberately reject a call by returning an error.
type Hooks struct {
	BeforePrompt []func(context.Context, string) (string, error)
	AfterModel   []func(context.Context, *agent.StepResult) error
	BeforeTool   []func(context.Context, *agent.ToolCall) error
	AfterTool    []func(context.Context, agent.ToolCall, *agent.ToolResult) error
}

// Loop repeatedly invokes the model until it stops requesting tools and no
// steering messages remain.
type Loop struct {
	config Config
}

var loopRunSequence atomic.Uint64

func New(config Config) (*Loop, error) {
	if config.Session == nil {
		return nil, errors.New("agentloop: session is nil")
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 32
	}
	if config.Tools == nil {
		config.Tools = map[string]Tool{}
	}
	return &Loop{config: config}, nil
}

// Run executes one user request through the full agentic loop.
func (l *Loop) Run(ctx context.Context, prompt string) (agent.Result, error) {
	return l.RunWithImages(ctx, prompt, nil)
}

// RunWithImages executes a user request that may include sandboxed image
// attachments. Images are sent only on the first model step.
func (l *Loop) RunWithImages(ctx context.Context, prompt string, images []agent.ImageInput) (agent.Result, error) {
	if strings.TrimSpace(prompt) == "" && len(images) == 0 {
		return agent.Result{}, errors.New("agentloop: prompt is empty")
	}
	l.emit(Event{Type: "agent_start"})
	runID := fmt.Sprintf("%s:run-%d", l.config.Session.ID(), loopRunSequence.Add(1))

	var err error
	if strings.TrimSpace(prompt) != "" {
		prompt, err = l.beforePrompt(ctx, prompt)
		if err != nil {
			return agent.Result{}, err
		}
	}
	var userMessages []string
	if strings.TrimSpace(prompt) != "" {
		userMessages = []string{prompt}
	}
	userImages := append([]agent.ImageInput(nil), images...)
	var toolResults []agent.ToolResult
	var last agent.Result
	var usage agent.Usage
	definitions := l.toolDefinitions()

	for stepNumber := 1; stepNumber <= l.config.MaxSteps; stepNumber++ {
		if err := ctx.Err(); err != nil {
			return agent.Result{}, err
		}
		messageID := fmt.Sprintf("%s:step-%d", runID, stepNumber)
		reasoningID := messageID + ":reasoning"
		reasoningText := ""
		reasoningStarted, reasoningEnded := false, false
		l.emit(Event{Type: "turn_start", Step: stepNumber})
		l.emit(Event{Type: "message_start", Step: stepNumber, MessageID: messageID})

		step, err := l.config.Session.Step(ctx, agent.StepRequest{
			UserMessages:    userMessages,
			Images:          userImages,
			ToolResults:     toolResults,
			Tools:           definitions,
			ReasoningEffort: l.config.ReasoningEffort,
			OnEvent: func(event agent.ModelEvent) {
				switch event.Type {
				case "text_delta":
					l.emit(Event{Type: "message_update", Step: stepNumber, MessageID: messageID, Delta: event.Delta})
				case "reasoning_delta":
					if !reasoningStarted {
						reasoningStarted = true
						l.emit(Event{Type: "reasoning_start", Step: stepNumber, MessageID: reasoningID})
					}
					reasoningText += event.Delta
					l.emit(Event{Type: "reasoning_update", Step: stepNumber, MessageID: reasoningID, Delta: event.Delta})
				case "reasoning_done":
					if strings.TrimSpace(event.Text) != "" {
						reasoningText = event.Text
					}
					if strings.TrimSpace(reasoningText) != "" {
						if !reasoningStarted {
							reasoningStarted = true
							l.emit(Event{Type: "reasoning_start", Step: stepNumber, MessageID: reasoningID})
						}
						reasoningEnded = true
						l.emit(Event{Type: "reasoning_end", Step: stepNumber, MessageID: reasoningID, Text: reasoningText})
					}
				}
			},
		})
		if err != nil {
			return agent.Result{}, err
		}
		if reasoningStarted && !reasoningEnded && strings.TrimSpace(reasoningText) != "" {
			l.emit(Event{Type: "reasoning_end", Step: stepNumber, MessageID: reasoningID, Text: reasoningText})
		}
		for _, hook := range l.config.Hooks.AfterModel {
			if err := hook(ctx, &step); err != nil {
				return agent.Result{}, fmt.Errorf("agentloop: after model hook: %w", err)
			}
		}
		last = step.Result
		usage = aggregateUsage(usage, step.Usage)
		last.Usage = usage
		stepUsage := step.Usage
		l.emit(Event{Type: "message_end", Step: stepNumber, MessageID: messageID, Text: step.Text, Usage: &stepUsage})

		toolResults = toolResults[:0]
		for _, call := range step.ToolCalls {
			result := agent.ToolResult{CallID: call.ID}
			l.emit(Event{
				Type:       "tool_execution_start",
				Step:       stepNumber,
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Arguments:  call.Arguments,
			})
			for _, hook := range l.config.Hooks.BeforeTool {
				if hookErr := hook(ctx, &call); hookErr != nil {
					result.IsError = true
					result.Output = hookErr.Error()
					break
				}
			}
			tool, ok := l.config.Tools[call.Name]
			if result.IsError {
				// A capability hook vetoed this call.
			} else if !ok {
				result.IsError = true
				result.Output = "unknown or disabled tool: " + call.Name
			} else {
				var toolOutput string
				toolOutput, err = tool.Execute(ctx, call.Arguments)
				result.Output = toolOutput
				if err != nil {
					result.IsError = true
					if result.Output != "" {
						result.Output += "\n"
					}
					result.Output += err.Error()
				}
			}
			for _, hook := range l.config.Hooks.AfterTool {
				if hookErr := hook(ctx, call, &result); hookErr != nil {
					result.IsError = true
					if result.Output != "" {
						result.Output += "\n"
					}
					result.Output += hookErr.Error()
				}
			}
			toolResults = append(toolResults, result)
			l.emit(Event{
				Type:       "tool_execution_end",
				Step:       stepNumber,
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Arguments:  call.Arguments,
				Output:     result.Output,
				IsError:    result.IsError,
			})
		}

		userMessages = nil
		userImages = nil
		if l.config.TakeSteering != nil {
			userMessages = l.config.TakeSteering()
			for index := range userMessages {
				userMessages[index], err = l.beforePrompt(ctx, userMessages[index])
				if err != nil {
					return agent.Result{}, err
				}
			}
		}
		l.emit(Event{Type: "turn_end", Step: stepNumber})

		if len(step.ToolCalls) == 0 && len(userMessages) == 0 {
			l.emit(Event{Type: "agent_end", Step: stepNumber, Text: last.Text})
			return last, nil
		}
	}

	return agent.Result{}, fmt.Errorf("agentloop: exceeded maximum of %d model steps", l.config.MaxSteps)
}

func (l *Loop) beforePrompt(ctx context.Context, prompt string) (string, error) {
	var err error
	for _, hook := range l.config.Hooks.BeforePrompt {
		prompt, err = hook(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("agentloop: before prompt hook: %w", err)
		}
	}
	return prompt, nil
}

func aggregateUsage(total, current agent.Usage) agent.Usage {
	total.InputTokens += current.InputTokens
	total.OutputTokens += current.OutputTokens
	total.CacheReadTokens += current.CacheReadTokens
	total.CacheWriteTokens += current.CacheWriteTokens
	currentTotal := current.TotalTokens
	if currentTotal == 0 {
		currentTotal = current.InputTokens + current.OutputTokens
	}
	total.TotalTokens += currentTotal
	currentContext := current.ContextTokens
	if currentContext == 0 {
		currentContext = current.InputTokens + current.OutputTokens
	}
	if currentContext != 0 {
		total.ContextTokens = currentContext
	}
	if current.ContextWindow != 0 {
		total.ContextWindow = current.ContextWindow
	}
	return total
}

func (l *Loop) toolDefinitions() []agent.ToolDefinition {
	names := make([]string, 0, len(l.config.Tools))
	for name := range l.config.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	definitions := make([]agent.ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, l.config.Tools[name].Definition())
	}
	return definitions
}

func (l *Loop) emit(event Event) {
	if l.config.Emit != nil {
		l.config.Emit(event)
	}
}
