package daemon

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	projectServerPath      = "/project/server/"
	projectProxyBootstrap  = "__dire_agent_project_proxy.js"
	maxProjectRewriteBytes = 64 << 20
)

//go:embed project_proxy_bootstrap.js
var projectProxyBootstrapJS []byte

var projectProxyTransport http.RoundTripper = &http.Transport{
	Proxy:                 nil,
	DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:     false,
	MaxIdleConns:          64,
	MaxIdleConnsPerHost:   8,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: time.Second,
}

var (
	htmlRootAttributePattern = regexp.MustCompile(`(?i)\b(?:src|href|action|poster)\s*=\s*["']?(/)`)
	javaScriptImportPattern  = regexp.MustCompile(`(?m)(\bfrom\s*|\bimport\s*(?:\(\s*)?)(["'])(/[^"']*)`)
)

type projectProxyRoute struct {
	Port         int
	Prefix       string
	UpstreamPath string
	NeedsSlash   bool
}

type projectProxyMode uint8

const (
	projectProxyStripPrefix projectProxyMode = iota
	projectProxyPreservePrefix
)

func (s *Server) handleProjectServer(writer http.ResponseWriter, request *http.Request) {
	route, err := parseProjectProxyRoute(request.URL.Path)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if route.NeedsSlash {
		location := route.Prefix + "/"
		if request.URL.RawQuery != "" {
			location += "?" + request.URL.RawQuery
		}
		http.Redirect(writer, request, location, http.StatusTemporaryRedirect)
		return
	}
	if s.ProjectProxyBlockedPort != 0 && route.Port == s.ProjectProxyBlockedPort {
		http.Error(writer, "refusing to proxy to the Dire Agent daemon port", http.StatusForbidden)
		return
	}
	if route.UpstreamPath == "/"+projectProxyBootstrap {
		serveProjectProxyBootstrap(writer, request)
		return
	}

	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(route.Port))}
	mode := s.detectProjectProxyMode(request.Context(), target, route.Prefix, route.Port)
	upstreamPath := route.UpstreamPath
	if mode == projectProxyPreservePrefix {
		upstreamPath = route.Prefix
		if route.UpstreamPath != "/" {
			upstreamPath += route.UpstreamPath
		}
	}
	publicOrigin := requestOrigin(request)
	proxy := &httputil.ReverseProxy{
		Transport:     projectProxyTransport,
		FlushInterval: -1,
		Rewrite: func(proxyRequest *httputil.ProxyRequest) {
			proxyRequest.SetURL(target)
			proxyRequest.Out.URL.Path = upstreamPath
			proxyRequest.Out.URL.RawPath = ""
			proxyRequest.Out.Header.Del("Forwarded")
			proxyRequest.Out.Header.Del("X-Forwarded-For")
			proxyRequest.Out.Header.Del("X-Forwarded-Host")
			proxyRequest.Out.Header.Del("X-Forwarded-Proto")
			proxyRequest.Out.Header.Del("X-Forwarded-Prefix")
			proxyRequest.SetXForwarded()
			proxyRequest.Out.Header.Set("X-Forwarded-Prefix", route.Prefix)
			proxyRequest.Out.Header.Set("X-Forwarded-Uri", proxyRequest.In.URL.RequestURI())
			if _, port, splitErr := net.SplitHostPort(proxyRequest.In.Host); splitErr == nil {
				proxyRequest.Out.Header.Set("X-Forwarded-Port", port)
			}
			rewriteProjectRequestHeader(proxyRequest.Out.Header, "Origin", publicOrigin, target.String(), route.Prefix)
			refererTarget := target.String()
			if mode == projectProxyPreservePrefix {
				refererTarget += route.Prefix
			}
			rewriteProjectRequestHeader(proxyRequest.Out.Header, "Referer", publicOrigin, refererTarget, route.Prefix)
			proxyRequest.Out.Header.Del("Accept-Encoding")
			proxyRequest.Out.Header.Del("If-Modified-Since")
			proxyRequest.Out.Header.Del("If-None-Match")
		},
		ModifyResponse: func(response *http.Response) error {
			rewriteProjectResponseHeaders(response.Header, target, publicOrigin, route.Prefix, route.Port)
			return rewriteProjectResponseBody(response, route.Prefix, route.UpstreamPath, mode)
		},
		ErrorHandler: func(response http.ResponseWriter, _ *http.Request, proxyErr error) {
			response.Header().Set("Content-Type", "text/plain; charset=utf-8")
			response.Header().Set("Cache-Control", "no-store")
			response.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprintf(response, "project server on localhost:%d is unavailable: %v\n", route.Port, proxyErr)
		},
	}
	proxy.ServeHTTP(writer, request)
}

func (s *Server) detectProjectProxyMode(ctx context.Context, target *url.URL, prefix string, port int) projectProxyMode {
	if cached, ok := s.projectProxyModes.Load(port); ok {
		return cached.(projectProxyMode)
	}
	mode := projectProxyStripPrefix
	probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	probeURL := *target
	probeURL.Path = prefix
	probe, err := http.NewRequestWithContext(probeContext, http.MethodHead, probeURL.String(), nil)
	if err == nil {
		probe.Header.Set("User-Agent", "dire-agent-project-proxy-probe")
		response, roundTripErr := projectProxyTransport.RoundTrip(probe)
		if roundTripErr == nil {
			_ = response.Body.Close()
			poweredBy := strings.ToLower(response.Header.Get("X-Powered-By"))
			vary := strings.ToLower(response.Header.Get("Vary"))
			looksLikeNext := strings.Contains(poweredBy, "next.js") || strings.Contains(vary, "next-router") || strings.Contains(vary, "rsc")
			if response.StatusCode < http.StatusBadRequest && looksLikeNext {
				mode = projectProxyPreservePrefix
			}
		}
	}
	actual, _ := s.projectProxyModes.LoadOrStore(port, mode)
	return actual.(projectProxyMode)
}

func parseProjectProxyRoute(requestPath string) (projectProxyRoute, error) {
	if requestPath == "/project/server" || requestPath == projectServerPath {
		return projectProxyRoute{}, errors.New("project server URL requires a port")
	}
	if !strings.HasPrefix(requestPath, projectServerPath) {
		return projectProxyRoute{}, errors.New("invalid project server URL")
	}
	remainder := strings.TrimPrefix(requestPath, projectServerPath)
	portText, suffix, hasSlash := strings.Cut(remainder, "/")
	if portText == "" {
		return projectProxyRoute{}, errors.New("project server URL requires a port")
	}
	for _, character := range portText {
		if character < '0' || character > '9' {
			return projectProxyRoute{}, errors.New("project server port must be numeric")
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return projectProxyRoute{}, errors.New("project server port must be between 1 and 65535")
	}
	if strconv.Itoa(port) != portText {
		return projectProxyRoute{}, errors.New("project server port must use canonical decimal notation")
	}
	prefix := projectServerPath + portText
	upstreamPath := "/"
	if hasSlash {
		upstreamPath += suffix
	}
	return projectProxyRoute{Port: port, Prefix: prefix, UpstreamPath: upstreamPath, NeedsSlash: !hasSlash}, nil
}

func serveProjectProxyBootstrap(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Length", strconv.Itoa(len(projectProxyBootstrapJS)))
	if request.Method == http.MethodGet {
		_, _ = writer.Write(projectProxyBootstrapJS)
	}
}

func requestOrigin(request *http.Request) string {
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + request.Host
}

func rewriteProjectRequestHeader(header http.Header, name, publicOrigin, targetOrigin, prefix string) {
	value := header.Get(name)
	if value == "" {
		return
	}
	if value == publicOrigin {
		header.Set(name, targetOrigin)
		return
	}
	header.Set(name, strings.ReplaceAll(value, publicOrigin+prefix, targetOrigin))
}

func rewriteProjectResponseHeaders(header http.Header, target *url.URL, publicOrigin, prefix string, port int) {
	for _, name := range []string{"Location", "Content-Location"} {
		for index, value := range header.Values(name) {
			header[name][index] = rewriteProjectResponseURL(value, publicOrigin, prefix, port)
		}
	}
	for _, name := range []string{"Link", "Refresh", "Content-Security-Policy", "Content-Security-Policy-Report-Only", "Report-To", "NEL"} {
		values := header.Values(name)
		if len(values) == 0 {
			continue
		}
		header.Del(name)
		for _, value := range values {
			value = replaceLoopbackOrigins(value, publicOrigin+prefix, port)
			value = rewriteQuotedRootPaths(value, prefix)
			value = strings.ReplaceAll(value, "</", "<"+prefix+"/")
			header.Add(name, value)
		}
	}
	if allowedOrigin := header.Get("Access-Control-Allow-Origin"); allowedOrigin != "" && allowedOrigin != "*" {
		if isLoopbackOrigin(allowedOrigin, port) || allowedOrigin == target.String() {
			header.Set("Access-Control-Allow-Origin", publicOrigin)
		}
	}
	cookies := header.Values("Set-Cookie")
	if len(cookies) != 0 {
		header.Del("Set-Cookie")
		for _, cookie := range cookies {
			header.Add("Set-Cookie", rewriteProjectCookie(cookie, prefix))
		}
	}
	header.Set("Cache-Control", "no-store")
}

func rewriteProjectResponseURL(value, publicOrigin, prefix string, port int) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//") {
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+"/") {
			return value
		}
		return prefix + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return replaceLoopbackOrigins(value, publicOrigin+prefix, port)
	}
	if parsed.IsAbs() && isLoopbackHost(parsed.Hostname()) && effectiveURLPort(parsed) == port {
		public, publicErr := url.Parse(publicOrigin)
		if publicErr != nil {
			return value
		}
		parsed.Scheme = public.Scheme
		parsed.Host = public.Host
		parsed.Path = prefix + ensureLeadingSlash(parsed.Path)
		return parsed.String()
	}
	return replaceLoopbackOrigins(value, publicOrigin+prefix, port)
}

func rewriteProjectCookie(value, prefix string) string {
	parts := strings.Split(value, ";")
	result := make([]string, 0, len(parts)+1)
	result = append(result, strings.TrimSpace(parts[0]))
	sawPath := false
	for _, part := range parts[1:] {
		attribute := strings.TrimSpace(part)
		lower := strings.ToLower(attribute)
		switch {
		case strings.HasPrefix(lower, "path="):
			sawPath = true
			cookiePath := strings.TrimSpace(attribute[len("path="):])
			if cookiePath == "" {
				cookiePath = "/"
			}
			if !strings.HasPrefix(cookiePath, "/") {
				cookiePath = "/" + cookiePath
			}
			if cookiePath != prefix && !strings.HasPrefix(cookiePath, prefix+"/") {
				cookiePath = prefix + cookiePath
			}
			result = append(result, "Path="+cookiePath)
		case strings.HasPrefix(lower, "domain="):
			domain := strings.Trim(strings.TrimSpace(attribute[len("domain="):]), ".")
			if !isLoopbackHost(domain) {
				result = append(result, attribute)
			}
		default:
			result = append(result, attribute)
		}
	}
	if !sawPath {
		result = append(result, "Path="+prefix+"/")
	}
	return strings.Join(result, "; ")
}

func rewriteProjectResponseBody(response *http.Response, prefix, upstreamPath string, mode projectProxyMode) error {
	if response.StatusCode == http.StatusSwitchingProtocols || response.Body == nil || !rewritableProxyContentType(response.Header.Get("Content-Type")) {
		return nil
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mode == projectProxyPreservePrefix && mediaType != "text/html" {
		return nil
	}
	if strings.HasPrefix(upstreamPath, "/_next/") && mediaType != "text/html" && mediaType != "text/css" {
		return nil
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return nil
	}
	original := response.Body
	contents, err := io.ReadAll(io.LimitReader(original, maxProjectRewriteBytes+1))
	if err != nil {
		return err
	}
	if len(contents) > maxProjectRewriteBytes {
		response.Body = &proxyReadCloser{Reader: io.MultiReader(bytes.NewReader(contents), original), Closer: original}
		return nil
	}
	if err := original.Close(); err != nil {
		return err
	}
	rewritten := rewriteProxyText(contents, prefix, mediaType, upstreamPath, mode == projectProxyStripPrefix)
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-MD5")
	response.Header.Del("ETag")
	return nil
}

func rewritableProxyContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	if mediaType == "text/event-stream" || mediaType == "text/x-component" {
		return false
	}
	return strings.HasPrefix(mediaType, "text/") ||
		strings.Contains(mediaType, "javascript") ||
		strings.Contains(mediaType, "json") ||
		strings.Contains(mediaType, "xml") ||
		mediaType == "application/manifest+json"
}

func rewriteProxyText(contents []byte, prefix, mediaType, upstreamPath string, rewriteURLs bool) []byte {
	text := string(contents)
	if rewriteURLs {
		switch {
		case mediaType == "text/html":
			text = rewriteJavaScriptImports(rewriteHTMLRootAttributes(text, prefix), prefix)
		case strings.Contains(mediaType, "javascript") && upstreamPath == "/@vite/client":
			text = restoreViteHMRModuleIDs(rewriteQuotedRootPaths(text, prefix), prefix)
		case strings.Contains(mediaType, "javascript"):
			text = rewriteJavaScriptImports(text, prefix)
		case mediaType == "text/css":
			text = rewriteUnquotedRootMarker(text, "url(", prefix)
		case mediaType == "application/manifest+json":
			text = rewriteQuotedRootPaths(text, prefix)
		}
	}
	if mediaType != "text/html" {
		return []byte(text)
	}
	bootstrap := `<meta name="dire-agent-proxy-prefix" content="` + prefix + `">`
	bootstrap += `<script src="` + prefix + `/` + projectProxyBootstrap + `"></script>`
	inlineBootstrap := strings.ReplaceAll(string(projectProxyBootstrapJS), "</script", `<\/script`)
	bootstrap += `<script>` + inlineBootstrap + `</script>`
	lower := strings.ToLower(text)
	if head := strings.Index(lower, "<head"); head >= 0 {
		if close := strings.Index(text[head:], ">"); close >= 0 {
			position := head + close + 1
			return []byte(text[:position] + bootstrap + text[position:])
		}
	}
	return []byte(bootstrap + text)
}

func rewriteHTMLRootAttributes(value, prefix string) string {
	return insertPrefixAtMatches(value, prefix, htmlRootAttributePattern, 1)
}

func rewriteJavaScriptImports(value, prefix string) string {
	return insertPrefixAtMatches(value, prefix, javaScriptImportPattern, 3)
}

func insertPrefixAtMatches(value, prefix string, pattern *regexp.Regexp, pathGroup int) string {
	matches := pattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value
	}
	var output strings.Builder
	output.Grow(len(value) + len(matches)*len(prefix))
	last := 0
	for _, match := range matches {
		pathStart := match[pathGroup*2]
		pathEnd := match[pathGroup*2+1]
		if pathStart < 0 || pathEnd <= pathStart {
			continue
		}
		output.WriteString(value[last:pathStart])
		path := value[pathStart:]
		if !strings.HasPrefix(path, "//") && !pathHasProxyPrefix(path, prefix) {
			output.WriteString(prefix)
		}
		output.WriteString(value[pathStart:pathEnd])
		last = pathEnd
	}
	output.WriteString(value[last:])
	return output.String()
}

func rewriteUnquotedRootMarker(value, marker, prefix string) string {
	needle := marker + "/"
	var output strings.Builder
	last := 0
	for {
		index := strings.Index(value[last:], needle)
		if index < 0 {
			break
		}
		pathStart := last + index + len(marker)
		output.WriteString(value[last:pathStart])
		path := value[pathStart:]
		if !strings.HasPrefix(path, "//") && !pathHasProxyPrefix(path, prefix) {
			output.WriteString(prefix)
		}
		output.WriteByte('/')
		last = pathStart + 1
	}
	if last == 0 {
		return value
	}
	output.WriteString(value[last:])
	return output.String()
}

func pathHasProxyPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	afterPrefix := path[len(prefix):]
	return afterPrefix == "" || strings.ContainsRune("/?#'\"`)&> ", rune(afterPrefix[0]))
}

// Vite's transformed modules use URL-like strings for two different purposes:
// browser imports need the public mount, while createHotContext/accept values
// are logical module IDs that must continue to match the unprefixed HMR
// payload sent by the upstream WebSocket server.
func restoreViteHMRModuleIDs(value, prefix string) string {
	for _, marker := range []string{"__vite__createHotContext(", "import.meta.hot.accept("} {
		searchFrom := 0
		for {
			start := strings.Index(value[searchFrom:], marker)
			if start < 0 {
				break
			}
			start += searchFrom + len(marker)
			end := strings.IndexByte(value[start:], ')')
			if end < 0 {
				break
			}
			end += start
			arguments := value[start:end]
			for _, quote := range []byte{'\'', '"', '`'} {
				arguments = strings.ReplaceAll(arguments, string(quote)+prefix+"/", string(quote)+"/")
			}
			value = value[:start] + arguments + value[end:]
			searchFrom = start + len(arguments) + 1
		}
	}
	return value
}

func rewriteQuotedRootPaths(value, prefix string) string {
	var output strings.Builder
	output.Grow(len(value) + 64)
	for index := 0; index < len(value); index++ {
		character := value[index]
		output.WriteByte(character)
		if character != '\'' && character != '"' && character != '`' {
			continue
		}
		if index+1 >= len(value) || value[index+1] != '/' || index+2 < len(value) && (value[index+2] == '/' || value[index+2] == '>') {
			continue
		}
		remainder := value[index+1:]
		if strings.HasPrefix(remainder, prefix) {
			afterPrefix := remainder[len(prefix):]
			if afterPrefix == "" || strings.ContainsRune("/?#'\"`", rune(afterPrefix[0])) {
				continue
			}
		}
		output.WriteString(prefix)
	}
	return output.String()
}

func replaceLoopbackOrigins(value, replacement string, port int) string {
	portText := strconv.Itoa(port)
	for _, origin := range []string{
		"http://127.0.0.1:" + portText,
		"http://localhost:" + portText,
		"http://[::1]:" + portText,
		"ws://127.0.0.1:" + portText,
		"ws://localhost:" + portText,
		"ws://[::1]:" + portText,
	} {
		value = strings.ReplaceAll(value, origin, replacement)
	}
	return value
}

func isLoopbackOrigin(value string, port int) bool {
	parsed, err := url.Parse(value)
	return err == nil && isLoopbackHost(parsed.Hostname()) && effectiveURLPort(parsed) == port
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func effectiveURLPort(value *url.URL) int {
	if port := value.Port(); port != "" {
		parsed, _ := strconv.Atoi(port)
		return parsed
	}
	if value.Scheme == "https" || value.Scheme == "wss" {
		return 443
	}
	return 80
}

func ensureLeadingSlash(value string) string {
	if value == "" {
		return "/"
	}
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

type proxyReadCloser struct {
	io.Reader
	io.Closer
}
