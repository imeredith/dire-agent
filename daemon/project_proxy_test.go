package daemon_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/dire-kiwi/dire-agent/daemon"
)

type proxiedRequest struct {
	Path             string
	Query            string
	Host             string
	Origin           string
	Referer          string
	ForwardedPrefix  string
	ForwardedURI     string
	ForwardedHost    string
	ForwardedProto   string
	ForwardedForSeen bool
}

func TestProjectServerProxyRewritesHTTPAndSupportsNestedRoutes(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []proxiedRequest
	)
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			http.NotFound(writer, request)
			return
		}
		mu.Lock()
		requests = append(requests, proxiedRequest{
			Path:             request.URL.Path,
			Query:            request.URL.RawQuery,
			Host:             request.Host,
			Origin:           request.Header.Get("Origin"),
			Referer:          request.Header.Get("Referer"),
			ForwardedPrefix:  request.Header.Get("X-Forwarded-Prefix"),
			ForwardedURI:     request.Header.Get("X-Forwarded-Uri"),
			ForwardedHost:    request.Header.Get("X-Forwarded-Host"),
			ForwardedProto:   request.Header.Get("X-Forwarded-Proto"),
			ForwardedForSeen: request.Header.Get("X-Forwarded-For") != "",
		})
		mu.Unlock()

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(writer, `<!doctype html><html><head><meta charSet="utf-8"/><script type="module" src="/@vite/client"></script><script type="module">import RefreshRuntime from "/@react-refresh"</script></head><body><a href=/settings>Settings</a><script>window.api = "/api"</script></body></html>`)
	}))
	port := testServerPort(t, upstream)
	proxy := newIPv4TestServer(t, (&daemon.Server{ProjectProxyEnabled: true}).Handler())
	prefix := fmt.Sprintf("/project/server/%d", port)

	redirectClient := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	redirectResponse, err := redirectClient.Get(proxy.URL + prefix + "?ready=1")
	if err != nil {
		t.Fatal(err)
	}
	defer redirectResponse.Body.Close()
	if redirectResponse.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("canonical redirect status = %d, want %d", redirectResponse.StatusCode, http.StatusTemporaryRedirect)
	}
	if location := redirectResponse.Header.Get("Location"); location != prefix+"/?ready=1" {
		t.Fatalf("canonical redirect = %q, want %q", location, prefix+"/?ready=1")
	}

	request, err := http.NewRequest(http.MethodGet, proxy.URL+prefix+"/nested/route?tab=files", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", proxy.URL)
	request.Header.Set("Referer", proxy.URL+prefix+"/previous")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("nested route status = %d, body = %s", response.StatusCode, contents)
	}
	body := string(contents)
	for _, want := range []string{
		`<meta name="dire-agent-proxy-prefix" content="` + prefix + `">`,
		`<script src="` + prefix + `/__dire_agent_project_proxy.js"></script>`,
		`const marker = "__DIRE_AGENT_PROJECT_PROXY__";`,
		`const legacyMarker = "__GOAGENT_PROJECT_PROXY__";`,
		`<meta charSet="utf-8"/>`,
		`src="` + prefix + `/@vite/client"`,
		`from "` + prefix + `/@react-refresh"`,
		`href=` + prefix + `/settings`,
		`window.api = "/api"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rewritten HTML does not contain %q:\n%s", want, body)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("upstream request count = %d, want 1: %#v", len(requests), requests)
	}
	got := requests[0]
	upstreamURL, _ := url.Parse(upstream.URL)
	if got.Path != "/nested/route" || got.Query != "tab=files" {
		t.Errorf("upstream URL = %s?%s, want /nested/route?tab=files", got.Path, got.Query)
	}
	if got.Host != upstreamURL.Host {
		t.Errorf("upstream Host = %q, want %q", got.Host, upstreamURL.Host)
	}
	if got.Origin != upstream.URL || got.Referer != upstream.URL+"/previous" {
		t.Errorf("rewritten Origin/Referer = %q / %q", got.Origin, got.Referer)
	}
	if got.ForwardedPrefix != prefix || got.ForwardedURI != prefix+"/nested/route?tab=files" {
		t.Errorf("forwarded prefix/URI = %q / %q", got.ForwardedPrefix, got.ForwardedURI)
	}
	proxyURL, _ := url.Parse(proxy.URL)
	if got.ForwardedHost != proxyURL.Host || got.ForwardedProto != "http" || !got.ForwardedForSeen {
		t.Errorf("forwarded host/proto/for = %q / %q / %v", got.ForwardedHost, got.ForwardedProto, got.ForwardedForSeen)
	}
}

func TestProjectServerProxyPreservesConfiguredNextBasePath(t *testing.T) {
	var (
		prefix       string
		upstreamPath string
	)
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead && request.URL.Path == prefix {
			writer.Header().Set("X-Powered-By", "Next.js")
			writer.WriteHeader(http.StatusOK)
			return
		}
		upstreamPath = request.URL.Path
		writer.Header().Set("X-Powered-By", "Next.js")
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(writer, `<html><head></head><body><a href="`+prefix+`/about">About</a><script>window.route = "/about"</script></body></html>`)
	}))
	port := testServerPort(t, upstream)
	prefix = fmt.Sprintf("/project/server/%d", port)
	proxy := newIPv4TestServer(t, (&daemon.Server{ProjectProxyEnabled: true}).Handler())

	request, err := http.NewRequest(http.MethodGet, proxy.URL+prefix+"/about", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/html")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("preserved Next response status = %d, body = %s", response.StatusCode, contents)
	}
	if upstreamPath != prefix+"/about" {
		t.Fatalf("Next upstream path = %q, want %q", upstreamPath, prefix+"/about")
	}
	if strings.Contains(string(contents), prefix+prefix) {
		t.Fatalf("Next basePath was doubled: %s", contents)
	}
	if !strings.Contains(string(contents), `window.route = "/about"`) {
		t.Fatalf("Next route state was rewritten instead of preserved: %s", contents)
	}
}

func TestProjectServerProxyRewritesRedirectsCookiesAndJavaScript(t *testing.T) {
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/redirect":
			writer.Header().Set("Location", "/login?from=dev")
			writer.Header().Add("Set-Cookie", "session=one; Path=/; Domain=127.0.0.1; HttpOnly; SameSite=Lax")
			writer.WriteHeader(http.StatusFound)
		case "/app.js":
			writer.Header().Set("Content-Type", "text/javascript")
			writer.Header().Set("ETag", `"upstream-etag"`)
			_, _ = io.WriteString(writer, `const root = "/"; const hmr = "/_next/webpack-hmr"; import("/src/main.js");`)
		case "/vite-module.js":
			writer.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(writer, `import { createHotContext as __vite__createHotContext } from "/@vite/client"; import.meta.hot = __vite__createHotContext("/main.js"); import value from "/value.js"; import.meta.hot.accept("/value.js", () => {});`)
		case "/mounted.js":
			writer.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(writer, `const mounted = "`+request.Header.Get("X-Forwarded-Prefix")+`";`)
		case "/_next/static/chunks/runtime.js":
			writer.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(writer, `const slash = "/"; const hmr = "/_next/webpack-hmr";`)
		default:
			http.NotFound(writer, request)
		}
	}))
	port := testServerPort(t, upstream)
	proxy := newIPv4TestServer(t, (&daemon.Server{ProjectProxyEnabled: true}).Handler())
	prefix := fmt.Sprintf("/project/server/%d", port)
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

	response, err := client.Get(proxy.URL + prefix + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("redirect status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	if location := response.Header.Get("Location"); location != prefix+"/login?from=dev" {
		t.Errorf("rewritten Location = %q", location)
	}
	cookie := response.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "Path="+prefix+"/") || strings.Contains(strings.ToLower(cookie), "domain=") {
		t.Errorf("rewritten Set-Cookie = %q", cookie)
	}

	response, err = client.Get(proxy.URL + prefix + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	javascript := string(contents)
	for _, want := range []string{
		`const root = "/"`,
		`const hmr = "/_next/webpack-hmr"`,
		`import("` + prefix + `/src/main.js")`,
	} {
		if !strings.Contains(javascript, want) {
			t.Errorf("rewritten JavaScript does not contain %q: %s", want, javascript)
		}
	}
	if response.Header.Get("ETag") != "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("rewritten cache headers = ETag %q, Cache-Control %q", response.Header.Get("ETag"), response.Header.Get("Cache-Control"))
	}

	response, err = client.Get(proxy.URL + prefix + "/vite-module.js")
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr = io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	viteModule := string(contents)
	for _, want := range []string{
		`from "` + prefix + `/@vite/client"`,
		`from "` + prefix + `/value.js"`,
		`__vite__createHotContext("/main.js")`,
		`import.meta.hot.accept("/value.js"`,
	} {
		if !strings.Contains(viteModule, want) {
			t.Errorf("rewritten Vite module does not contain %q: %s", want, viteModule)
		}
	}

	response, err = client.Get(proxy.URL + prefix + "/mounted.js")
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr = io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := string(contents); got != `const mounted = "`+prefix+`";` {
		t.Errorf("already-mounted path was rewritten again: %s", got)
	}

	response, err = client.Get(proxy.URL + prefix + "/_next/static/chunks/runtime.js")
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr = io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := string(contents); got != `const slash = "/"; const hmr = "/_next/webpack-hmr";` {
		t.Errorf("Next internal JavaScript was semantically rewritten: %s", got)
	}
}

func TestProjectServerProxyBootstrapSupportsPushRouters(t *testing.T) {
	upstream := newIPv4TestServer(t, http.NotFoundHandler())
	port := testServerPort(t, upstream)
	proxy := newIPv4TestServer(t, (&daemon.Server{ProjectProxyEnabled: true}).Handler())
	prefix := fmt.Sprintf("/project/server/%d", port)

	response, err := http.Get(proxy.URL + prefix + "/__dire_agent_project_proxy.js")
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("bootstrap response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	script := string(contents)
	for _, marker := range []string{"__DIRE_AGENT_PROJECT_PROXY__", "__GOAGENT_PROJECT_PROXY__", "pushState", "replaceState", "WebSocket", "EventSource", "XMLHttpRequest", "MutationObserver"} {
		if !strings.Contains(script, marker) {
			t.Errorf("bootstrap is missing %q", marker)
		}
	}
}

func TestProjectServerProxyPassesViteAndNextWebSockets(t *testing.T) {
	type observedUpgrade struct {
		Path     string
		Origin   string
		Host     string
		Prefix   string
		Protocol string
		Message  string
	}
	observed := make(chan observedUpgrade, 2)
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{Subprotocols: []string{"vite-hmr"}})
		if err != nil {
			return
		}
		defer connection.CloseNow()
		ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		messageType, message, err := connection.Read(ctx)
		if err != nil {
			return
		}
		observed <- observedUpgrade{
			Path: request.URL.Path, Origin: request.Header.Get("Origin"), Host: request.Host,
			Prefix: request.Header.Get("X-Forwarded-Prefix"), Protocol: connection.Subprotocol(), Message: string(message),
		}
		_ = connection.Write(ctx, messageType, []byte("echo:"+string(message)))
	}))
	port := testServerPort(t, upstream)
	proxy := newIPv4TestServer(t, (&daemon.Server{ProjectProxyEnabled: true}).Handler())
	prefix := fmt.Sprintf("/project/server/%d", port)

	tests := []struct {
		name        string
		path        string
		subprotocol string
	}{
		{name: "Vite HMR", path: "/", subprotocol: "vite-hmr"},
		{name: "Next HMR", path: "/_next/webpack-hmr"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			options := &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{proxy.URL}}}
			if test.subprotocol != "" {
				options.Subprotocols = []string{test.subprotocol}
			}
			websocketURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + prefix + test.path
			connection, response, err := websocket.Dial(ctx, websocketURL, options)
			if err != nil {
				status := 0
				if response != nil {
					status = response.StatusCode
				}
				t.Fatalf("dial proxy WebSocket (status %d): %v", status, err)
			}
			defer connection.CloseNow()
			if got := connection.Subprotocol(); got != test.subprotocol {
				t.Fatalf("negotiated subprotocol = %q, want %q", got, test.subprotocol)
			}
			if err := connection.Write(ctx, websocket.MessageText, []byte(test.name)); err != nil {
				t.Fatal(err)
			}
			_, reply, err := connection.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if string(reply) != "echo:"+test.name {
				t.Fatalf("reply = %q", reply)
			}
			got := <-observed
			if got.Path != test.path || got.Origin != upstream.URL || got.Prefix != prefix || got.Protocol != test.subprotocol || got.Message != test.name {
				t.Errorf("upstream upgrade = %#v", got)
			}
			upstreamURL, _ := url.Parse(upstream.URL)
			if got.Host != upstreamURL.Host {
				t.Errorf("upstream WebSocket Host = %q, want %q", got.Host, upstreamURL.Host)
			}
		})
	}
}

func TestProjectServerProxyRejectsInvalidAndDaemonPorts(t *testing.T) {
	proxy := newIPv4TestServer(t, (&daemon.Server{ProjectProxyEnabled: true, ProjectProxyBlockedPort: 7331}).Handler())
	tests := []struct {
		path string
		want int
	}{
		{path: "/project/server", want: http.StatusBadRequest},
		{path: "/project/server/", want: http.StatusBadRequest},
		{path: "/project/server/nope/", want: http.StatusBadRequest},
		{path: "/project/server/0001/", want: http.StatusBadRequest},
		{path: "/project/server/65536/", want: http.StatusBadRequest},
		{path: "/project/server/7331/", want: http.StatusForbidden},
	}
	for _, test := range tests {
		response, err := http.Get(proxy.URL + test.path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != test.want {
			t.Errorf("GET %s = %d, want %d", test.path, response.StatusCode, test.want)
		}
	}
}

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func testServerPort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}
