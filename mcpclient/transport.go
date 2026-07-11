package mcpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/imeredith/dire-agent/internal/sandboxenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TransportFactory creates a fresh, single-use transport for a server.
type TransportFactory interface {
	NewTransport(context.Context, ServerConfig) (mcp.Transport, error)
}

// TransportFactoryFunc adapts a function into a TransportFactory.
type TransportFactoryFunc func(context.Context, ServerConfig) (mcp.Transport, error)

func (f TransportFactoryFunc) NewTransport(ctx context.Context, cfg ServerConfig) (mcp.Transport, error) {
	return f(ctx, cfg)
}

// DefaultTransportFactory builds stdio and Streamable HTTP transports. The
// optional HTTPClient is cloned before per-server headers are installed.
type DefaultTransportFactory struct {
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

func (f DefaultTransportFactory) NewTransport(_ context.Context, cfg ServerConfig) (mcp.Transport, error) {
	switch cfg.Transport {
	case TransportStdio:
		cmd := exec.Command(cfg.Command, cfg.Arguments...)
		cmd.Dir = cfg.WorkingDirectory
		if cfg.InheritEnvironment {
			cmd.Env = mergeEnvironment(os.Environ(), cfg.Environment)
		} else {
			base := []string{}
			if path := os.Getenv("PATH"); path != "" {
				base = append(base, "PATH="+path)
			}
			cmd.Env = mergeEnvironment(base, cfg.Environment)
		}
		if cfg.Sandboxed {
			cmd.Env = sandboxenv.Sanitize(cmd.Env)
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	case TransportStreamableHTTP:
		client := cloneHTTPClient(f.HTTPClient)
		endpoint, err := url.Parse(cfg.Endpoint)
		if err != nil {
			return nil, err
		}
		secureRedirects(client, endpoint)
		if len(cfg.Headers) > 0 {
			client.Transport = headerTransport{
				base: client.Transport, headers: cloneMap(cfg.Headers), scheme: endpoint.Scheme, host: endpoint.Host,
			}
		}
		limit := f.MaxResponseBytes
		if limit <= 0 {
			limit = 16 << 20
		}
		client.Transport = responseLimitTransport{base: client.Transport, max: limit}
		return &mcp.StreamableClientTransport{
			Endpoint:             cfg.Endpoint,
			HTTPClient:           client,
			MaxRetries:           cfg.MaxRetries,
			DisableStandaloneSSE: cfg.DisableStandaloneSSE,
		}, nil
	default:
		return nil, &ConfigError{Server: cfg.Name, Message: "unsupported transport"}
	}
}

func secureRedirects(client *http.Client, endpoint *url.URL) {
	previous := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("MCP HTTP redirect limit exceeded")
		}
		if request.URL.Scheme != endpoint.Scheme || request.URL.Host != endpoint.Host {
			return fmt.Errorf("MCP HTTP cross-origin redirect refused")
		}
		if previous != nil {
			return previous(request, via)
		}
		return nil
	}
}

type responseLimitTransport struct {
	base http.RoundTripper
	max  int64
}

func (t responseLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > t.max {
		response.Body.Close()
		return nil, fmt.Errorf("MCP HTTP response exceeds %d bytes", t.max)
	}
	response.Body = &limitedReadCloser{Reader: io.LimitReader(response.Body, t.max+1), closer: response.Body}
	return response, nil
}

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *limitedReadCloser) Close() error { return r.closer.Close() }

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
	scheme  string
	host    string
}

func (t headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copyRequest := request.Clone(request.Context())
	copyRequest.Header = request.Header.Clone()
	if request.URL.Scheme == t.scheme && request.URL.Host == t.host {
		for name, value := range t.headers {
			copyRequest.Header.Set(name, value)
		}
	}
	return t.base.RoundTrip(copyRequest)
}

func cloneHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	copyClient := *base
	if copyClient.Transport == nil {
		copyClient.Transport = http.DefaultTransport
	}
	return &copyClient
}

func mergeEnvironment(current []string, overrides map[string]string) []string {
	values := make(map[string]string, len(current)+len(overrides))
	for _, entry := range current {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
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
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}
