package client

import (
	"context"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

type LaunchProjectAppResult struct {
	Launched bool   `json:"launched"`
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
}

func (c *Client) ProjectLaunchers(ctx context.Context, projectID string) ([]configuration.ProjectLauncher, error) {
	var launchers []configuration.ProjectLauncher
	err := c.call(ctx, daemon.Command{Type: "get_project_launchers", ProjectID: projectID}, &launchers)
	return launchers, err
}

func (c *Client) LaunchProjectApp(ctx context.Context, projectID, launcherID string) (LaunchProjectAppResult, error) {
	var result LaunchProjectAppResult
	err := c.call(ctx, daemon.Command{Type: "launch_project_app", ProjectID: projectID, LauncherID: launcherID}, &result)
	return result, err
}

func (c *Client) InspectProjectWorkspace(ctx context.Context, projectID, folder string) (daemon.ProjectWorkspaceInspection, error) {
	var inspection daemon.ProjectWorkspaceInspection
	err := c.call(ctx, daemon.Command{Type: "inspect_project_workspace", ProjectID: projectID, Folder: folder}, &inspection)
	return inspection, err
}

func (c *Client) ProjectEnvironments(ctx context.Context, projectID, folder string) ([]daemon.ProjectEnvironment, error) {
	var environments []daemon.ProjectEnvironment
	err := c.call(ctx, daemon.Command{Type: "get_project_environments", ProjectID: projectID, Folder: folder}, &environments)
	return environments, err
}

func (c *Client) PutProjectEnvironment(ctx context.Context, projectID, folder string, environment daemon.ProjectEnvironment, expectedHash string) (daemon.ProjectEnvironment, error) {
	var saved daemon.ProjectEnvironment
	err := c.call(ctx, daemon.Command{
		Type: "put_project_environment", ProjectID: projectID, Folder: folder,
		Environment: &environment, EnvironmentID: environment.ID, ExpectedHash: expectedHash,
	}, &saved)
	return saved, err
}

func (c *Client) DeleteProjectEnvironment(ctx context.Context, projectID, folder, environmentID, expectedHash string) error {
	return c.call(ctx, daemon.Command{
		Type: "delete_project_environment", ProjectID: projectID, Folder: folder,
		EnvironmentID: environmentID, ExpectedHash: expectedHash,
	}, nil)
}

func (c *Client) CreateThread(ctx context.Context, options daemon.CreateThreadOptions) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "create_thread", Options: options}, &thread)
	return thread, err
}

func (c *Client) CreateProject(ctx context.Context, options daemon.CreateProjectOptions) (threadstore.Project, error) {
	var project threadstore.Project
	err := c.call(ctx, daemon.Command{Type: "create_project", Options: options}, &project)
	return project, err
}

// NewSession is a Pi-compatible name for CreateThread.
func (c *Client) NewSession(ctx context.Context, options daemon.CreateThreadOptions) (threadstore.Thread, error) {
	return c.CreateThread(ctx, options)
}

func (c *Client) ListThreads(ctx context.Context) ([]threadstore.Thread, error) {
	var threads []threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "list_threads"}, &threads)
	return threads, err
}

func (c *Client) ListProjects(ctx context.Context) ([]threadstore.Project, error) {
	var projects []threadstore.Project
	err := c.call(ctx, daemon.Command{Type: "list_projects"}, &projects)
	return projects, err
}

func (c *Client) Thread(ctx context.Context, threadID string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "get_thread", ProjectID: threadID, ThreadID: threadID}, &thread)
	return thread, err
}

func (c *Client) Project(ctx context.Context, projectID string) (threadstore.Project, error) {
	var project threadstore.Project
	err := c.call(ctx, daemon.Command{Type: "get_project", ProjectID: projectID}, &project)
	return project, err
}

func (c *Client) ProjectState(ctx context.Context, projectID string) (daemon.RuntimeState, error) {
	var state daemon.RuntimeState
	err := c.call(ctx, daemon.Command{Type: "get_project_state", ProjectID: projectID}, &state)
	return state, err
}

func (c *Client) ProjectMessages(ctx context.Context, projectID string, after int64, limit int) ([]threadstore.Message, error) {
	var messages []threadstore.Message
	err := c.call(ctx, daemon.Command{Type: "get_project_messages", ProjectID: projectID, After: after, Limit: limit}, &messages)
	return messages, err
}

func (c *Client) ProjectEvents(ctx context.Context, projectID string, after int64, limit int) ([]threadstore.Event, error) {
	var events []threadstore.Event
	err := c.call(ctx, daemon.Command{Type: "get_project_events", ProjectID: projectID, After: after, Limit: limit}, &events)
	return events, err
}

func (c *Client) State(ctx context.Context, threadID string) (daemon.RuntimeState, error) {
	var state daemon.RuntimeState
	err := c.call(ctx, daemon.Command{Type: "get_state", ProjectID: threadID, ThreadID: threadID}, &state)
	return state, err
}

func (c *Client) Messages(ctx context.Context, threadID string, after int64, limit int) ([]threadstore.Message, error) {
	var messages []threadstore.Message
	err := c.call(ctx, daemon.Command{Type: "get_messages", ProjectID: threadID, ThreadID: threadID, After: after, Limit: limit}, &messages)
	return messages, err
}

func (c *Client) HistoryEvents(ctx context.Context, threadID string, after int64, limit int) ([]threadstore.Event, error) {
	var events []threadstore.Event
	err := c.call(ctx, daemon.Command{Type: "get_events", ProjectID: threadID, ThreadID: threadID, After: after, Limit: limit}, &events)
	return events, err
}

func (c *Client) DeleteThread(ctx context.Context, threadID string) error {
	return c.call(ctx, daemon.Command{Type: "delete_thread", ProjectID: threadID, ThreadID: threadID}, nil)
}

func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	return c.call(ctx, daemon.Command{Type: "delete_project", ProjectID: projectID}, nil)
}
