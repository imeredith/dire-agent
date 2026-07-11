package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/threadstore"
)

// Prompt starts a run or queues the message according to streamingBehavior.
func (m *Manager) Prompt(ctx context.Context, id, message, streamingBehavior string) error {
	return m.PromptWithAttachments(ctx, id, message, streamingBehavior, nil)
}

// PromptWithAttachments starts a prompt with images copied into the current
// project's sandbox. Attachments are deliberately unavailable to pathless
// chats and to queued steering/follow-up messages.
func (m *Manager) PromptWithAttachments(ctx context.Context, id, message, streamingBehavior string, attachments []ImageAttachment) error {
	if strings.TrimSpace(message) == "" && len(attachments) == 0 {
		return errors.New("daemon: prompt is empty")
	}
	decoded, err := decodeImageAttachments(attachments)
	if err != nil {
		return err
	}
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return err
	}
	if err := m.refreshCapabilities(ctx, runtime); err != nil {
		return err
	}
	runtime.mu.Lock()
	if runtime.finishing {
		runtime.mu.Unlock()
		return errors.New("daemon: conversation is settling; retry after agent_settled")
	}
	if runtime.running {
		if len(decoded) != 0 {
			runtime.mu.Unlock()
			return errors.New("daemon: images can only be attached when starting a new prompt")
		}
		switch streamingBehavior {
		case "steer":
			runtime.steering = append(runtime.steering, message)
		case "followUp", "follow_up":
			runtime.followUps = append(runtime.followUps, message)
		default:
			runtime.mu.Unlock()
			return errors.New("daemon: conversation is running; streaming_behavior must be steer or followUp")
		}
		steeringCount, followCount := len(runtime.steering), len(runtime.followUps)
		runtime.mu.Unlock()
		m.emit(ctx, runtime, "queue_update", map[string]int{"steering": steeringCount, "follow_ups": followCount})
		return nil
	}
	storedAttachments, images, err := persistImageAttachments(runtime.thread, decoded)
	if err != nil {
		runtime.mu.Unlock()
		return err
	}
	runtime.running = true
	runtime.thread.Status = "running"
	runContext, cancel := context.WithCancel(context.Background())
	runtime.cancel = cancel
	runtime.runWG.Add(1)
	runtime.mu.Unlock()
	_ = runtime.persistThread(context.Background())
	go func() {
		defer runtime.runWG.Done()
		runtime.run(runContext, message, storedAttachments, images)
	}()
	return nil
}

func (m *Manager) Steer(ctx context.Context, id, message string) error {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	running := runtime.running
	runtime.mu.Unlock()
	if !running {
		return errors.New("daemon: cannot steer an idle conversation; use prompt")
	}
	return m.Prompt(ctx, id, message, "steer")
}

func (m *Manager) FollowUp(ctx context.Context, id, message string) error {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	running := runtime.running
	runtime.mu.Unlock()
	if !running {
		return m.Prompt(ctx, id, message, "")
	}
	return m.Prompt(ctx, id, message, "followUp")
}

func (m *Manager) Abort(ctx context.Context, id string) error {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	cancel := runtime.cancel
	if cancel == nil {
		runtime.mu.Unlock()
		return errors.New("daemon: conversation is not running")
	}
	cancel()
	runtime.mu.Unlock()
	m.emit(ctx, runtime, "abort_requested", map[string]bool{"requested": true})
	return nil
}

func (r *threadRuntime) run(ctx context.Context, prompt string, attachments []ImageAttachment, images []agent.ImageInput) {
	current := prompt
	var finalErr error
	var lastText string
	for current != "" || len(images) != 0 {
		var messageData json.RawMessage
		if len(attachments) != 0 {
			messageData, _ = json.Marshal(map[string]any{"attachments": attachments})
		}
		_, _ = r.db.AppendMessage(context.Background(), threadstore.Message{
			Kind: "message", Role: "user", Content: current, Data: messageData,
		})
		modelPrompt := current
		r.mu.Lock()
		preparePrompt := r.preparePrompt
		session := r.session
		toolset := r.tools
		thinkingLevel := r.thread.ThinkingLevel
		hooks := r.hooks
		r.mu.Unlock()
		var err error
		if preparePrompt != nil && strings.TrimSpace(current) != "" {
			modelPrompt, err = preparePrompt(ctx, current)
		}
		var loop *agentloop.Loop
		if err == nil {
			loop, err = agentloop.New(agentloop.Config{
				Session: session, Tools: toolset, ReasoningEffort: thinkingLevel,
				MaxSteps: r.manager.config.MaxAgentSteps, TakeSteering: r.takeSteering,
				Hooks: hooks,
				Emit:  func(event agentloop.Event) { r.manager.handleLoopEvent(r, event) },
			})
		}
		var result agent.Result
		if err == nil {
			result, err = loop.RunWithImages(ctx, modelPrompt, images)
		}
		attachments = nil
		images = nil
		if err != nil {
			r.manager.emit(context.Background(), r, "agent_error", map[string]string{"error": err.Error()})
			r.clearQueues()
			finalErr = err
			break
		}
		if result.Text != "" {
			lastText = result.Text
			_, _ = r.db.AppendMessage(context.Background(), threadstore.Message{Kind: "message", Role: "assistant", Content: result.Text})
		}
		if err := r.saveState(context.Background()); err != nil {
			r.manager.emit(context.Background(), r, "persistence_error", map[string]string{"error": err.Error()})
			finalErr = err
			break
		}
		current = r.takeFollowUpOrSettle()
	}

	r.mu.Lock()
	r.finishing = true
	r.running = false
	r.cancel = nil
	if r.thread.IsSubagent() {
		switch {
		case errors.Is(finalErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
			r.thread.Status = "interrupted"
		case finalErr != nil:
			r.thread.Status = "failed"
		default:
			r.thread.Status = "completed"
		}
	} else {
		r.thread.Status = "idle"
	}
	r.thread.UpdatedAt = time.Now().UTC()
	isSubagent := r.thread.IsSubagent()
	r.mu.Unlock()
	_ = r.persistThread(context.Background())
	if isSubagent {
		r.manager.reportAgentCompletion(r, lastText, finalErr)
	} else {
		r.manager.notifyTeam(teamRootID(r.snapshotThread()))
	}
	r.manager.emit(context.Background(), r, "agent_settled", map[string]bool{"settled": true})
	r.mu.Lock()
	r.finishing = false
	r.mu.Unlock()
}

func (r *threadRuntime) takeSteering() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return takeQueue(&r.steering, r.thread.SteeringMode)
}

func (r *threadRuntime) takeFollowUpOrSettle() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	messages := takeQueue(&r.followUps, r.thread.FollowUpMode)
	if len(messages) == 0 {
		r.running = false
		r.cancel = nil
		r.thread.Status = "idle"
		r.thread.UpdatedAt = time.Now().UTC()
	}
	return strings.Join(messages, "\n\n")
}

func takeQueue(queue *[]string, mode string) []string {
	if len(*queue) == 0 {
		return nil
	}
	if mode == "all" {
		messages := append([]string(nil), (*queue)...)
		*queue = nil
		return messages
	}
	message := (*queue)[0]
	*queue = (*queue)[1:]
	return []string{message}
}

func (r *threadRuntime) clearQueues() {
	r.mu.Lock()
	r.steering = nil
	r.followUps = nil
	r.mu.Unlock()
}
