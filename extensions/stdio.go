package extensions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/imeredith/dire-agent/internal/sandboxenv"
)

// StdioConnector launches commands directly with exec.CommandContext. It does
// not invoke a shell or interpret Command or Args.
type StdioConnector struct{}

func (StdioConnector) Connect(ctx context.Context, spec ProcessSpec, limits Limits) (Connection, error) {
	if err := validateProcessSpec(spec, false); err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(lifetime, spec.Command, spec.Args...)
	command.Dir = spec.Dir
	command.Env = processEnvironment(spec)
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		cancel()
		return nil, err
	}
	connection := &rpcConnection{
		stdin: stdin, stdout: stdout, cancel: cancel, command: command,
		limits: limits, pending: map[string]chan rpcResponse{}, done: make(chan struct{}),
		stderr: newBoundedBuffer(limits.MaxStderrBytes),
	}
	go func() { _, _ = io.Copy(connection.stderr, stderr) }()
	go connection.readLoop()
	go func() { connection.finish(command.Wait()) }()
	return connection, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("extension RPC error %d: %s", e.Code, e.Message)
}

type rpcConnection struct {
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	command    *exec.Cmd
	cancel     context.CancelFunc
	limits     Limits
	stderr     *boundedBuffer
	nextID     atomic.Uint64
	writeMu    sync.Mutex
	mu         sync.Mutex
	pending    map[string]chan rpcResponse
	failure    error
	done       chan struct{}
	finishOnce sync.Once
	closeOnce  sync.Once
}

func (c *rpcConnection) Call(ctx context.Context, method string, params any, result any) error {
	if strings.TrimSpace(method) == "" {
		return errors.New("extensions: RPC method is required")
	}
	id := c.nextID.Add(1)
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if len(payload)+1 > c.limits.MaxMessageBytes {
		return fmt.Errorf("extensions: RPC request exceeds %d bytes", c.limits.MaxMessageBytes)
	}
	key := fmt.Sprint(id)
	response := make(chan rpcResponse, 1)
	if err := c.register(key, response); err != nil {
		return err
	}
	defer c.unregister(key)
	c.writeMu.Lock()
	written, err := c.stdin.Write(append(payload, '\n'))
	c.writeMu.Unlock()
	if err == nil && written != len(payload)+1 {
		err = io.ErrShortWrite
	}
	if err != nil {
		c.finish(err)
		return err
	}
	select {
	case reply := <-response:
		return decodeResponse(reply, result)
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		select {
		case reply := <-response:
			return decodeResponse(reply, result)
		default:
		}
		return c.connectionError()
	}
}

func decodeResponse(reply rpcResponse, result any) error {
	if reply.Error != nil {
		return reply.Error
	}
	if result == nil || len(reply.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(reply.Result, result); err != nil {
		return fmt.Errorf("extensions: decode RPC result: %w", err)
	}
	return nil
}

func (c *rpcConnection) readLoop() {
	scanner := bufio.NewScanner(c.stdout)
	initial := min(64<<10, c.limits.MaxMessageBytes)
	scanner.Buffer(make([]byte, initial), c.limits.MaxMessageBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var response rpcResponse
		if err := json.Unmarshal(line, &response); err != nil {
			c.finish(fmt.Errorf("extensions: invalid RPC response: %w", err))
			return
		}
		if response.JSONRPC != "2.0" || len(response.ID) == 0 {
			continue
		}
		key := strings.TrimSpace(string(response.ID))
		c.mu.Lock()
		pending := c.pending[key]
		c.mu.Unlock()
		if pending != nil {
			select {
			case pending <- response:
			default:
			}
		}
	}
	if err := scanner.Err(); err != nil {
		c.finish(err)
	} else {
		c.finish(io.EOF)
	}
}

func (c *rpcConnection) register(id string, response chan rpcResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return c.connectionErrorLocked()
	default:
	}
	c.pending[id] = response
	return nil
}

func (c *rpcConnection) unregister(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *rpcConnection) finish(err error) {
	c.finishOnce.Do(func() {
		c.mu.Lock()
		c.failure = err
		c.mu.Unlock()
		c.cancel()
		close(c.done)
	})
}

func (c *rpcConnection) connectionError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectionErrorLocked()
}

func (c *rpcConnection) connectionErrorLocked() error {
	if c.failure == nil {
		return ErrClosed
	}
	return fmt.Errorf("%w: %v", ErrClosed, c.failure)
}

func (c *rpcConnection) Stderr() string { return c.stderr.String() }

func (c *rpcConnection) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		c.cancel()
	})
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func mergedEnvironment(overrides map[string]string, inherit bool) []string {
	values := map[string]string{}
	if path := os.Getenv("PATH"); path != "" {
		values["PATH"] = path
	}
	if inherit {
		for _, entry := range os.Environ() {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				values[key] = value
			}
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func processEnvironment(spec ProcessSpec) []string {
	environment := mergedEnvironment(spec.Env, spec.InheritEnv)
	if spec.Sandboxed {
		environment = sandboxenv.Sanitize(environment)
	}
	return environment
}
