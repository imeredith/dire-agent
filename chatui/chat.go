// Package chatui provides the interactive Bubble Tea client for the daemon.
package chatui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/imeredith/dire-agent/agentteam"
	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

// API is the daemon client surface used by the chat UI.
type API interface {
	Events() <-chan daemon.WireEvent
	State(context.Context, string) (daemon.RuntimeState, error)
	Messages(context.Context, string, int64, int) ([]threadstore.Message, error)
	Subscribe(context.Context, string) error
	Unsubscribe(context.Context, string) error
	Prompt(context.Context, string, string, string) error
	Steer(context.Context, string, string) error
	FollowUp(context.Context, string, string) error
	Abort(context.Context, string) error
	SetModel(context.Context, string, string) (threadstore.Thread, error)
	SetThinkingLevel(context.Context, string, string) (threadstore.Thread, error)
	SetThreadName(context.Context, string, string) (threadstore.Thread, error)
	SetProjectAdditionalFolders(context.Context, string, []string) (threadstore.Project, error)
	SpawnAgent(context.Context, agentteam.SpawnRequest) (agentteam.Agent, error)
	ListAgents(context.Context, string) ([]agentteam.Agent, error)
	SendAgentMessage(context.Context, string, string, string, bool) (agentteam.Message, error)
	WaitAgents(context.Context, string, []string, time.Duration) (agentteam.WaitResult, error)
	InterruptAgent(context.Context, string, string) error
	DeleteAgent(context.Context, string, string) error
	CapabilityCommands(context.Context, string) ([]daemon.CapabilityCommandInfo, error)
	ExecuteCapabilityCommand(context.Context, string, string, string) (capability.CommandResult, error)
}

type Options struct {
	ConversationID string
	ChatID         string
	ProjectID      string
	// ThreadID is a deprecated compatibility alias for ProjectID.
	ThreadID      string
	InitialPrompt string
}

// Run loads a project or standalone chat and runs an interactive full-screen UI.
func Run(ctx context.Context, api API, options Options) error {
	if api == nil {
		return errors.New("chatui: client is required")
	}
	conversationID := firstSet(options.ConversationID, options.ChatID, options.ProjectID, options.ThreadID)
	if conversationID == "" {
		return errors.New("chatui: conversation id is required")
	}
	if err := api.Subscribe(ctx, conversationID); err != nil {
		return fmt.Errorf("chatui: subscribe: %w", err)
	}
	defer func() {
		unsubscribeContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = api.Unsubscribe(unsubscribeContext, conversationID)
	}()
	state, err := api.State(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("chatui: load state: %w", err)
	}
	messages, err := api.Messages(ctx, conversationID, 0, 10000)
	if err != nil {
		return fmt.Errorf("chatui: load messages: %w", err)
	}

	program := tea.NewProgram(newModel(ctx, api, state, messages, strings.TrimSpace(options.InitialPrompt)), tea.WithContext(ctx))
	_, err = program.Run()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, tea.ErrInterrupted) {
		return fmt.Errorf("chatui: run: %w", err)
	}
	return nil
}

func firstSet(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
