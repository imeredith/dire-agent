package client

import (
	"context"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func (c *Client) Config(ctx context.Context) (configuration.Config, error) {
	var config configuration.Config
	err := c.call(ctx, daemon.Command{Type: "config_get"}, &config)
	return config, err
}

func (c *Client) UpdateConfig(ctx context.Context, config configuration.Config) (configuration.Config, error) {
	var updated configuration.Config
	err := c.call(ctx, daemon.Command{
		Type: "config_update", Config: &config, ExpectedRevision: config.Revision,
	}, &updated)
	return updated, err
}

func (c *Client) ValidateConfig(ctx context.Context, config configuration.Config) error {
	return c.call(ctx, daemon.Command{Type: "config_validate", Config: &config}, nil)
}

type EffectiveConfig struct {
	Settings        configuration.Settings `json:"settings"`
	ProjectOverride bool                   `json:"project_override"`
}

func (c *Client) EffectiveConfig(ctx context.Context, conversationID string) (EffectiveConfig, error) {
	var result EffectiveConfig
	err := c.call(ctx, daemon.Command{
		Type: "config_effective", ConversationID: conversationID,
	}, &result)
	return result, err
}

func (c *Client) ProjectSandbox(ctx context.Context, projectID string) (daemon.ProjectSandboxSettings, error) {
	var result daemon.ProjectSandboxSettings
	err := c.call(ctx, daemon.Command{Type: "get_project_sandbox", ProjectID: projectID}, &result)
	return result, err
}

// SetProjectSandbox sets a project's sandbox mode. A nil mode makes the
// project inherit the global default again.
func (c *Client) SetProjectSandbox(ctx context.Context, projectID string, mode *configuration.SandboxMode) (daemon.ProjectSandboxSettings, error) {
	requested := "inherit"
	if mode != nil {
		requested = string(*mode)
	}
	var result daemon.ProjectSandboxSettings
	err := c.call(ctx, daemon.Command{Type: "set_project_sandbox", ProjectID: projectID, Sandbox: requested}, &result)
	return result, err
}

func (c *Client) Capabilities(ctx context.Context, conversationID string) (daemon.CapabilityState, error) {
	var result daemon.CapabilityState
	err := c.call(ctx, daemon.Command{
		Type: "get_capabilities", ConversationID: conversationID,
	}, &result)
	return result, err
}
