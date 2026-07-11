package mcpclient

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TransportKind identifies an MCP transport supported by the client.
type TransportKind string

const (
	TransportStdio          TransportKind = "stdio"
	TransportStreamableHTTP TransportKind = "streamable_http"
)

// ServerConfig contains connection settings for one MCP server. Enabled and
// Trusted are deliberately separate: neither an untrusted nor a disabled
// server is contacted.
type ServerConfig struct {
	Name                 string            `json:"name"`
	Enabled              bool              `json:"enabled"`
	Trusted              bool              `json:"trusted"`
	Transport            TransportKind     `json:"transport"`
	Command              string            `json:"command,omitempty"`
	Arguments            []string          `json:"arguments,omitempty"`
	Environment          map[string]string `json:"environment,omitempty"`
	InheritEnvironment   bool              `json:"inherit_environment,omitempty"`
	WorkingDirectory     string            `json:"working_directory,omitempty"`
	Endpoint             string            `json:"endpoint,omitempty"`
	Headers              map[string]string `json:"headers,omitempty"`
	MaxRetries           int               `json:"max_retries,omitempty"`
	DisableStandaloneSSE bool              `json:"disable_standalone_sse,omitempty"`
	ConnectTimeout       time.Duration     `json:"connect_timeout,omitempty"`
	ListTimeout          time.Duration     `json:"list_timeout,omitempty"`
	CallTimeout          time.Duration     `json:"call_timeout,omitempty"`
}

// Options configures the client without changing individual server settings.
type Options struct {
	TransportFactory   TransportFactory
	Connector          Connector
	ClientName         string
	ClientVersion      string
	ConnectTimeout     time.Duration
	ListTimeout        time.Duration
	CallTimeout        time.Duration
	MaxResultBytes     int
	MaxStructuredBytes int
}

func (c ServerConfig) validate() error {
	if err := validateName(c.Name); err != nil {
		return fmt.Errorf("server name: %w", err)
	}
	for _, timeout := range []time.Duration{c.ConnectTimeout, c.ListTimeout, c.CallTimeout} {
		if timeout < 0 {
			return errors.New("timeouts cannot be negative")
		}
	}
	switch c.Transport {
	case TransportStdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio command is required")
		}
		for key, value := range c.Environment {
			if key == "" || strings.ContainsAny(key, "=\x00") {
				return fmt.Errorf("invalid environment key %q", key)
			}
			if strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("environment value for %q contains NUL", key)
			}
		}
	case TransportStreamableHTTP:
		u, err := url.Parse(c.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("streamable HTTP endpoint must be an http or https URL")
		}
		for name, value := range c.Headers {
			if name == "" || http.CanonicalHeaderKey(name) == "" || strings.ContainsAny(name, "\r\n") {
				return fmt.Errorf("invalid HTTP header name %q", name)
			}
			if strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("HTTP header %q contains a newline", name)
			}
		}
	default:
		return fmt.Errorf("unsupported transport %q", c.Transport)
	}
	return nil
}

func normalizeOptions(options Options) Options {
	if options.ClientName == "" {
		options.ClientName = "dire-agent"
	}
	if options.ClientVersion == "" {
		options.ClientVersion = "dev"
	}
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = 15 * time.Second
	}
	if options.ListTimeout <= 0 {
		options.ListTimeout = 15 * time.Second
	}
	if options.CallTimeout <= 0 {
		options.CallTimeout = 60 * time.Second
	}
	if options.MaxResultBytes <= 0 {
		options.MaxResultBytes = 64 << 10
	}
	if options.MaxStructuredBytes <= 0 {
		options.MaxStructuredBytes = 16 << 10
	}
	return options
}

func cloneConfig(c ServerConfig) ServerConfig {
	c.Arguments = append([]string(nil), c.Arguments...)
	c.Environment = cloneMap(c.Environment)
	c.Headers = cloneMap(c.Headers)
	return c
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
