package mcpclient

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDefaultTransportFactoryBuildsStdio(t *testing.T) {
	factory := DefaultTransportFactory{}
	transport, err := factory.NewTransport(context.Background(), ServerConfig{
		Name: "local", Transport: TransportStdio, Command: "server", Arguments: []string{"--stdio"},
		Environment: map[string]string{"MCP_TEST_VALUE": "set"}, WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}
	stdio, ok := transport.(*mcp.CommandTransport)
	if !ok {
		t.Fatalf("transport type = %T", transport)
	}
	if stdio.Command.Path != "server" || len(stdio.Command.Args) != 2 || stdio.Command.Dir != "/tmp" {
		t.Fatalf("unexpected command: %#v", stdio.Command)
	}
	found := false
	leaked := false
	for _, entry := range stdio.Command.Env {
		found = found || entry == "MCP_TEST_VALUE=set"
		leaked = leaked || strings.HasPrefix(entry, "HOME=")
	}
	if !found {
		t.Fatalf("override missing from environment: %#v", stdio.Command.Env)
	}
	if leaked {
		t.Fatalf("stdio inherited the daemon environment without permission: %#v", stdio.Command.Env)
	}
}

func TestSandboxedStdioStripsLoaderEnvironment(t *testing.T) {
	t.Setenv("LD_PRELOAD", "./project-owned.so")
	factory := DefaultTransportFactory{}
	transport, err := factory.NewTransport(context.Background(), ServerConfig{
		Name: "local", Transport: TransportStdio, Command: "sandbox-wrapper",
		Environment:        map[string]string{"DYLD_INSERT_LIBRARIES": "./project-owned.dylib", "SAFE": "value"},
		InheritEnvironment: true, Sandboxed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := transport.(*mcp.CommandTransport).Command
	for _, entry := range command.Env {
		if strings.HasPrefix(entry, "LD_") || strings.HasPrefix(entry, "DYLD_") {
			t.Fatalf("sandbox wrapper inherited loader control: %q", entry)
		}
	}
	foundSafe := false
	for _, entry := range command.Env {
		foundSafe = foundSafe || entry == "SAFE=value"
	}
	if !foundSafe {
		t.Fatalf("safe environment was removed: %#v", command.Env)
	}
}

func TestHTTPTransportRefusesCrossOriginRedirectAndOversizedResponse(t *testing.T) {
	factory := DefaultTransportFactory{
		MaxResponseBytes: 2,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK, ContentLength: 3,
				Body: io.NopCloser(strings.NewReader("abc")), Header: http.Header{},
			}, nil
		})},
	}
	transport, err := factory.NewTransport(context.Background(), ServerConfig{
		Name: "remote", Transport: TransportStreamableHTTP, Endpoint: "https://example.test/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := transport.(*mcp.StreamableClientTransport).HTTPClient
	redirect := &http.Request{URL: &url.URL{Scheme: "https", Host: "other.test"}}
	if err := client.CheckRedirect(redirect, []*http.Request{{URL: &url.URL{Scheme: "https", Host: "example.test"}}}); err == nil {
		t.Fatal("cross-origin redirect was accepted")
	}
	request, _ := http.NewRequest(http.MethodGet, "https://example.test/mcp", nil)
	if _, err := client.Do(request); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestDefaultTransportFactoryInjectsHTTPHeaders(t *testing.T) {
	var requests int
	base := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		got := request.Header.Get("Authorization")
		if request.URL.Host == "example.test" && got != "Bearer token" {
			t.Fatalf("same-origin Authorization = %q", got)
		}
		if request.URL.Host != "example.test" && got != "" {
			t.Fatalf("cross-origin Authorization leaked as %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: http.Header{}}, nil
	})
	factory := DefaultTransportFactory{HTTPClient: &http.Client{Transport: base}}
	transport, err := factory.NewTransport(context.Background(), ServerConfig{
		Name: "remote", Transport: TransportStreamableHTTP, Endpoint: "https://example.test/mcp",
		Headers: map[string]string{"Authorization": "Bearer token"}, MaxRetries: -1, DisableStandaloneSSE: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpTransport, ok := transport.(*mcp.StreamableClientTransport)
	if !ok {
		t.Fatalf("transport type = %T", transport)
	}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/check", nil)
	response, err := httpTransport.HTTPClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	request, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, "https://redirected.test/check", nil)
	response, err = httpTransport.HTTPClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests != 2 {
		t.Fatalf("round trips = %d", requests)
	}
	if !httpTransport.DisableStandaloneSSE || httpTransport.MaxRetries != -1 {
		t.Fatalf("transport options not preserved: %#v", httpTransport)
	}
}

func TestModelNameAndConfigValidation(t *testing.T) {
	name, err := ModelName("git", "read-file.v2")
	if err != nil || name != "mcp__git__read-file.v2" {
		t.Fatalf("ModelName = %q, %v", name, err)
	}
	if _, err := ModelName("bad server", "tool"); err == nil {
		t.Fatal("invalid server name accepted")
	}
	_, err = New([]ServerConfig{{Name: "bad", Enabled: true, Trusted: true, Transport: TransportStreamableHTTP,
		Endpoint: "file:///tmp/socket"}}, Options{})
	if err == nil {
		t.Fatal("invalid HTTP endpoint accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
