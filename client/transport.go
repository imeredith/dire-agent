package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/coder/websocket/wsjson"

	"github.com/imeredith/dire-agent/daemon"
)

func (c *Client) call(ctx context.Context, command daemon.Command, destination any) error {
	if command.ID == "" {
		command.ID = fmt.Sprintf("req-%d", c.nextID.Add(1))
	}
	responseChannel := make(chan wireResponse, 1)
	c.pendingMu.Lock()
	select {
	case <-c.done:
		c.pendingMu.Unlock()
		return c.Err()
	default:
		c.pending[command.ID] = responseChannel
		c.pendingMu.Unlock()
	}

	c.writeMu.Lock()
	err := wsjson.Write(ctx, c.connection, command)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(command.ID)
		return fmt.Errorf("client: write command: %w", err)
	}

	select {
	case response := <-responseChannel:
		if !response.Success {
			return errors.New(response.Error)
		}
		if destination != nil && len(response.Data) != 0 && string(response.Data) != "null" {
			if err := json.Unmarshal(response.Data, destination); err != nil {
				return fmt.Errorf("client: decode %s response: %w", command.Type, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.removePending(command.ID)
		return ctx.Err()
	case <-c.done:
		c.removePending(command.ID)
		return c.Err()
	}
}

func (c *Client) readLoop() {
	defer close(c.done)
	defer close(c.events)
	for {
		var raw json.RawMessage
		if err := wsjson.Read(c.ctx, c.connection, &raw); err != nil {
			c.fail(err)
			return
		}
		var envelope struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			continue
		}
		if envelope.Type == "response" {
			var response wireResponse
			if json.Unmarshal(raw, &response) == nil {
				c.pendingMu.Lock()
				channel := c.pending[response.ID]
				delete(c.pending, response.ID)
				c.pendingMu.Unlock()
				if channel != nil {
					channel <- response
				}
			}
			continue
		}
		var event daemon.WireEvent
		if json.Unmarshal(raw, &event) == nil {
			select {
			case c.events <- event:
			case <-c.ctx.Done():
				return
			}
		}
	}
}

func (c *Client) removePending(id string) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *Client) fail(err error) {
	c.errMu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.errMu.Unlock()
	c.cancel()
	c.pendingMu.Lock()
	for id, channel := range c.pending {
		delete(c.pending, id)
		channel <- wireResponse{Success: false, Error: err.Error()}
	}
	c.pendingMu.Unlock()
}
