// Package controlapp implements the Dire Agent control client command.
package controlapp

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/imeredith/dire-agent/chatui"
	"github.com/imeredith/dire-agent/client"
	"github.com/imeredith/dire-agent/daemon"
)

// Run executes the control client with the supplied command-line arguments.
func Run(arguments []string) error {
	flags := flag.NewFlagSet("dire-agent tui", flag.ContinueOnError)
	url := flags.String("url", "ws://127.0.0.1:7331/ws", "daemon WebSocket URL")
	action := flags.String("action", "chat", "chat, prompt, steer, follow-up, abort, list, list-chats, state, create, or create-chat")
	var projectID string
	flags.StringVar(&projectID, "project", "", "project id")
	flags.StringVar(&projectID, "thread", "", "deprecated alias for -project")
	var chatID string
	flags.StringVar(&chatID, "chat", "", "standalone chat id")
	flags.StringVar(&chatID, "conversation", "", "conversation id")
	standalone := flags.Bool("standalone", false, "create a chat without a project folder")
	model := flags.String("model", "", "model when creating a project")
	var folder string
	flags.StringVar(&folder, "folder", "", "folder when creating a project")
	flags.StringVar(&folder, "cwd", "", "alias for -folder")
	name := flags.String("name", "", "name when creating a project")
	thinking := flags.String("thinking", "", "thinking level when creating a project")
	toolNames := flags.String("tools", "", "comma-separated tools when creating a project")
	worktree := flags.Bool("worktree", false, "create the project in a managed Git worktree")
	baseRef := flags.String("base-ref", "HEAD", "Git ref used as the worktree starting point")
	environmentID := flags.String("environment", "", "repo-local environment ID used to set up a new worktree")
	sourceProjectID := flags.String("source-project", "", "existing project whose settings the worktree inherits")
	message := flags.String("message", "", "initial chat message or one-shot action text")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if !*worktree && (strings.TrimSpace(*environmentID) != "" || strings.TrimSpace(*sourceProjectID) != "" || strings.TrimSpace(*baseRef) != "HEAD") {
		return errors.New("-base-ref, -environment, and -source-project require -worktree")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectionContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	daemonClient, err := client.Dial(connectionContext, *url)
	if err != nil {
		return err
	}
	defer daemonClient.Close()
	messageText := strings.TrimSpace(*message)
	if messageText == "" {
		messageText = strings.TrimSpace(strings.Join(flags.Args(), " "))
	}
	createOptions := daemon.CreateThreadOptions{
		Name: *name, Model: *model, CWD: folder, ThinkingLevel: *thinking, Tools: splitCSV(*toolNames),
	}
	if *worktree {
		createOptions.Worktree = &daemon.CreateWorktreeOptions{
			BaseRef: strings.TrimSpace(*baseRef), EnvironmentID: strings.TrimSpace(*environmentID),
			SourceProjectID: strings.TrimSpace(*sourceProjectID),
		}
	}
	chatOptions := daemon.CreateChatOptions{
		Name: *name, Model: *model, ThinkingLevel: *thinking,
	}
	resourceID := projectID
	if chatID != "" {
		resourceID = chatID
	}

	switch *action {
	case "chat":
		if resourceID == "" && *standalone {
			chat, err := daemonClient.CreateChat(ctx, chatOptions)
			if err != nil {
				return err
			}
			resourceID = chat.ID
		} else if resourceID == "" {
			project, err := daemonClient.CreateProject(ctx, createOptions)
			if err != nil {
				return err
			}
			resourceID = project.ID
		}
		return chatui.Run(ctx, daemonClient, chatui.Options{ConversationID: resourceID, InitialPrompt: messageText})
	case "list":
		projects, err := daemonClient.ListProjects(ctx)
		return printJSON(projects, err)
	case "list-chats":
		chats, err := daemonClient.ListChats(ctx)
		return printJSON(chats, err)
	case "create":
		project, err := daemonClient.CreateProject(ctx, createOptions)
		return printJSON(project, err)
	case "create-chat":
		chat, err := daemonClient.CreateChat(ctx, chatOptions)
		return printJSON(chat, err)
	case "state":
		if resourceID == "" {
			return errors.New("-project, -chat, or -conversation is required")
		}
		state, err := daemonClient.State(ctx, resourceID)
		return printJSON(state, err)
	case "abort":
		if resourceID == "" {
			return errors.New("a conversation id is required")
		}
		return daemonClient.Abort(ctx, resourceID)
	case "steer":
		if resourceID == "" || messageText == "" {
			return errors.New("a conversation id and message text are required")
		}
		return daemonClient.Steer(ctx, resourceID, messageText)
	case "follow-up":
		if resourceID == "" || messageText == "" {
			return errors.New("a conversation id and message text are required")
		}
		return daemonClient.FollowUp(ctx, resourceID, messageText)
	case "prompt":
		if messageText == "" {
			return errors.New("prompt text is required")
		}
		if resourceID == "" && *standalone {
			chat, err := daemonClient.CreateChat(ctx, chatOptions)
			if err != nil {
				return err
			}
			resourceID = chat.ID
			fmt.Fprintln(os.Stderr, "chat:", chat.ID)
		} else if resourceID == "" {
			project, err := daemonClient.CreateProject(ctx, createOptions)
			if err != nil {
				return err
			}
			resourceID = project.ID
			fmt.Fprintln(os.Stderr, "project:", project.ID)
		}
		if err := daemonClient.Prompt(ctx, resourceID, messageText, ""); err != nil {
			return err
		}
		return printRunEvents(ctx, daemonClient, resourceID)
	default:
		return fmt.Errorf("unknown action %q", *action)
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func printRunEvents(ctx context.Context, daemonClient *client.Client, conversationID string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-daemonClient.Events():
			if event.ConversationID != conversationID && event.ChatID != conversationID && event.ProjectID != conversationID && event.ThreadID != conversationID {
				continue
			}
			switch event.Type {
			case "message_update":
				var data struct {
					Delta string `json:"delta"`
				}
				_ = json.Unmarshal(event.Data, &data)
				fmt.Print(data.Delta)
			case "tool_execution_start":
				var data struct {
					ToolName string `json:"tool_name"`
				}
				_ = json.Unmarshal(event.Data, &data)
				fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", data.ToolName)
			case "agent_error":
				return fmt.Errorf("agent failed: %s", event.Data)
			case "agent_settled":
				fmt.Println()
				return nil
			}
		}
	}
}

func printJSON(value any, err error) error {
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
