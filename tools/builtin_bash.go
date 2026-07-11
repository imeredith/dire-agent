package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/imeredith/dire-agent/agentloop"
)

func bashTool(root string, executor shellExecutor) agentloop.Tool {
	return functionTool{
		definition: definition("bash", "Run a shell command from the main project folder in the platform sandbox. Configured additional folders are writable by absolute path; network and all other writes are denied.", `{"type":"object","properties":{"command":{"type":"string"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":300}},"required":["command"],"additionalProperties":false}`),
		execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Command        string `json:"command"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			if input.TimeoutSeconds <= 0 {
				input.TimeoutSeconds = 30
			}
			commandContext, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutSeconds)*time.Second)
			defer cancel()
			var output limitedBuffer
			err := executor.Run(commandContext, root, input.Command, &output)
			if commandContext.Err() == context.DeadlineExceeded {
				return output.String(), fmt.Errorf("command timed out after %d seconds", input.TimeoutSeconds)
			}
			if err != nil {
				return output.String(), fmt.Errorf("command failed: %w", err)
			}
			return output.String(), nil
		},
	}
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := maxToolOutput - b.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.Buffer.Write(data)
	}
	return original, nil
}
