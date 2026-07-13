// Package client is a Pi-inspired Go client for the dire-agent daemon WebSocket API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/dire-kiwi/dire-agent/daemon"
)

type Client struct {
	connection *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	writeMu    sync.Mutex
	nextID     atomic.Uint64
	pendingMu  sync.Mutex
	pending    map[string]chan wireResponse
	events     chan daemon.WireEvent
	done       chan struct{}
	errMu      sync.Mutex
	err        error
}

type wireResponse struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func Dial(ctx context.Context, url string) (*Client, error) {
	connection, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("client: connect: %w", err)
	}
	clientContext, cancel := context.WithCancel(context.Background())
	client := &Client{
		connection: connection, ctx: clientContext, cancel: cancel,
		pending: make(map[string]chan wireResponse), events: make(chan daemon.WireEvent, 1024),
		done: make(chan struct{}),
	}
	go client.readLoop()
	return client, nil
}

func (c *Client) Events() <-chan daemon.WireEvent { return c.events }
func (c *Client) Done() <-chan struct{}           { return c.done }

func (c *Client) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.err
}

func (c *Client) Close() error {
	c.cancel()
	err := c.connection.Close(websocket.StatusNormalClosure, "")
	<-c.done
	return err
}
