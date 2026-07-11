package agentloop_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentloop"
)

type fakeSession struct {
	steps int
}

func (s *fakeSession) ID() string { return "session-1" }
func (s *fakeSession) Run(context.Context, string) (agent.Result, error) {
	return agent.Result{}, nil
}
func (s *fakeSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.steps++
	if s.steps == 1 {
		return agent.StepResult{
			Result: agent.Result{
				SessionID: s.ID(),
				TurnID:    "turn-1",
				Usage: agent.Usage{
					InputTokens: 100, OutputTokens: 10, CacheReadTokens: 60,
					CacheWriteTokens: 20, TotalTokens: 110,
					ContextTokens: 110, ContextWindow: 372_000,
				},
			},
			ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"value":"x"}`)}},
		}, nil
	}
	if len(request.ToolResults) != 1 || request.ToolResults[0].Output != "found x" {
		panic("tool result was not returned to model")
	}
	if request.OnEvent != nil {
		request.OnEvent(agent.ModelEvent{Type: "reasoning_delta", Delta: "Checking the tool result."})
		request.OnEvent(agent.ModelEvent{Type: "reasoning_done", Text: "Checking the tool result."})
		request.OnEvent(agent.ModelEvent{Type: "text_delta", Delta: "done"})
	}
	return agent.StepResult{Result: agent.Result{
		Text: "done", SessionID: s.ID(), TurnID: "turn-2",
		Usage: agent.Usage{
			InputTokens: 140, OutputTokens: 20, CacheReadTokens: 90,
			CacheWriteTokens: 5, TotalTokens: 160,
			ContextTokens: 160, ContextWindow: 372_000,
		},
	}}, nil
}

type fakeTool struct{}

func (fakeTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}
}
func (fakeTool) Execute(_ context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Value string `json:"value"`
	}
	_ = json.Unmarshal(arguments, &input)
	return "found " + input.Value, nil
}

func TestLoopExecutesToolsAndStreamsEvents(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	var events []agentloop.Event
	loop, err := agentloop.New(agentloop.Config{
		Session: session,
		Tools:   map[string]agentloop.Tool{"lookup": fakeTool{}},
		Emit:    func(event agentloop.Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := loop.Run(context.Background(), "find it")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "done" || session.steps != 2 {
		t.Fatalf("result/steps = %#v/%d", result, session.steps)
	}
	wantUsage := agent.Usage{
		InputTokens: 240, OutputTokens: 30, CacheReadTokens: 150,
		CacheWriteTokens: 25, TotalTokens: 270,
		ContextTokens: 160, ContextWindow: 372_000,
	}
	if result.Usage != wantUsage {
		t.Fatalf("result usage = %#v, want %#v", result.Usage, wantUsage)
	}
	wantTypes := map[string]bool{
		"agent_start": true, "turn_start": true, "message_update": true,
		"reasoning_start": true, "reasoning_update": true, "reasoning_end": true,
		"tool_execution_start": true, "tool_execution_end": true, "agent_end": true,
	}
	for _, event := range events {
		delete(wantTypes, event.Type)
	}
	if len(wantTypes) != 0 {
		t.Fatalf("missing event types: %#v", wantTypes)
	}
	var messageEnds []agentloop.Event
	for _, event := range events {
		if event.Type == "message_end" {
			messageEnds = append(messageEnds, event)
		}
	}
	if len(messageEnds) != 2 || messageEnds[0].Usage == nil || messageEnds[1].Usage == nil {
		t.Fatalf("message_end usage events = %#v", messageEnds)
	}
	if messageEnds[0].Usage.InputTokens != 100 || messageEnds[1].Usage.InputTokens != 140 {
		t.Fatalf("message_end usages = %#v / %#v", messageEnds[0].Usage, messageEnds[1].Usage)
	}
	var reasoningEnd, toolEnd *agentloop.Event
	for index := range events {
		switch events[index].Type {
		case "reasoning_end":
			reasoningEnd = &events[index]
		case "tool_execution_end":
			toolEnd = &events[index]
		}
	}
	if reasoningEnd == nil || reasoningEnd.Text != "Checking the tool result." {
		t.Fatalf("reasoning end = %#v", reasoningEnd)
	}
	if toolEnd == nil || string(toolEnd.Arguments) != `{"value":"x"}` {
		t.Fatalf("tool end arguments = %#v", toolEnd)
	}
}

func TestLoopConsumesSteeringBeforeSettling(t *testing.T) {
	t.Parallel()

	session := &steeringSession{}
	steered := false
	loop, err := agentloop.New(agentloop.Config{
		Session: session,
		TakeSteering: func() []string {
			if steered {
				return nil
			}
			steered = true
			return []string{"new direction"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background(), "initial"); err != nil {
		t.Fatal(err)
	}
	if session.steps != 2 || session.secondMessage != "new direction" {
		t.Fatalf("steps/message = %d/%q", session.steps, session.secondMessage)
	}
}

func TestLoopMessageIDsAreUniqueAcrossRuns(t *testing.T) {
	t.Parallel()
	session := &steeringSession{}
	var messageIDs []string
	loop, err := agentloop.New(agentloop.Config{
		Session: session,
		Emit: func(event agentloop.Event) {
			if event.Type == "message_start" {
				messageIDs = append(messageIDs, event.MessageID)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	if len(messageIDs) != 2 || messageIDs[0] == messageIDs[1] {
		t.Fatalf("message ids = %#v", messageIDs)
	}
}

type steeringSession struct {
	steps         int
	secondMessage string
}

func (s *steeringSession) ID() string { return "steering" }
func (s *steeringSession) Run(context.Context, string) (agent.Result, error) {
	return agent.Result{}, nil
}
func (s *steeringSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.steps++
	if s.steps == 2 && len(request.UserMessages) != 0 {
		s.secondMessage = request.UserMessages[0]
	}
	return agent.StepResult{Result: agent.Result{Text: "ok"}}, nil
}
