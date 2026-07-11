package client

import (
	"context"

	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/daemon"
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

func (c *Client) Capabilities(ctx context.Context, conversationID string) (daemon.CapabilityState, error) {
	var result daemon.CapabilityState
	err := c.call(ctx, daemon.Command{
		Type: "get_capabilities", ConversationID: conversationID,
	}, &result)
	return result, err
}
