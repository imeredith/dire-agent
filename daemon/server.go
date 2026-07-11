package daemon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/imeredith/dire-agent/configuration"
)

type Server struct {
	Manager                 *Manager
	Config                  *configuration.Store
	WebUI                   fs.FS
	ProjectProxyEnabled     bool
	ProjectProxyBlockedPort int
	projectProxyModes       sync.Map
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/attachments/", s.handleAttachment)
	mux.HandleFunc("/terminal", s.handleTerminal)
	if s.ProjectProxyEnabled {
		mux.HandleFunc("/project/server", s.handleProjectServer)
		mux.HandleFunc("/project/server/", s.handleProjectServer)
	}
	if s.WebUI != nil {
		mux.HandleFunc("/", s.handleWebUI)
	}
	return mux
}

func (s *Server) handleWebUI(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, ok := webUIPath(request.URL.Path)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if info, err := fs.Stat(s.WebUI, name); err == nil {
		if info.Mode().IsRegular() {
			s.serveWebUIFile(writer, request, name, false)
		} else {
			http.NotFound(writer, request)
		}
		return
	}
	if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" || !acceptsHTML(request) {
		http.NotFound(writer, request)
		return
	}
	s.serveWebUIFile(writer, request, "index.html", true)
}

func (s *Server) serveWebUIFile(writer http.ResponseWriter, request *http.Request, name string, spaFallback bool) {
	contents, err := fs.ReadFile(s.WebUI, name)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	info, err := fs.Stat(s.WebUI, name)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if name == "index.html" || spaFallback {
		writer.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		writer.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.ServeContent(writer, request, path.Base(name), info.ModTime(), bytes.NewReader(contents))
}

func webUIPath(requestPath string) (string, bool) {
	if requestPath == "" || requestPath == "/" {
		return "index.html", true
	}
	name := strings.TrimPrefix(requestPath, "/")
	if !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

func acceptsHTML(request *http.Request) bool {
	accept := request.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func (s *Server) handleAttachment(writer http.ResponseWriter, request *http.Request) {
	if s.Manager == nil {
		http.Error(writer, "daemon is not initialized", http.StatusServiceUnavailable)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(request.URL.Path, "/attachments/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || filepath.Base(parts[1]) != parts[1] {
		http.NotFound(writer, request)
		return
	}
	project, err := s.Manager.Project(request.Context(), parts[0])
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	path, info, found := existingAttachmentPath(project.CWD, parts[1])
	if !found {
		http.NotFound(writer, request)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	defer file.Close()
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		var header [512]byte
		count, _ := io.ReadFull(file, header[:])
		contentType = http.DetectContentType(header[:count])
		_, _ = file.Seek(0, io.SeekStart)
	}
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(writer, fmt.Sprintf("unsupported attachment type %q", contentType), http.StatusUnsupportedMediaType)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	writer.Header().Set("Content-Disposition", "inline")
	http.ServeContent(writer, request, parts[1], info.ModTime(), file)
}

func existingAttachmentPath(projectRoot, name string) (string, os.FileInfo, bool) {
	for _, directory := range []string{".dire-agent", ".goagent"} {
		namespace := filepath.Join(projectRoot, directory)
		attachments := filepath.Join(namespace, "attachments")
		if !safeAttachmentDirectory(namespace) || !safeAttachmentDirectory(attachments) {
			continue
		}
		path := filepath.Join(attachments, name)
		info, err := os.Lstat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return path, info, true
		}
	}
	return "", nil, false
}

func safeAttachmentDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func (s *Server) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	if s.Manager == nil {
		http.Error(writer, "daemon is not initialized", http.StatusServiceUnavailable)
		return
	}
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(16 << 20)

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	outbound := make(chan any, 1024)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-outbound:
				writeContext, writeCancel := context.WithTimeout(ctx, 10*time.Second)
				err := wsjson.Write(writeContext, connection, message)
				writeCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	client := &serverClient{
		manager: s.Manager, config: s.Config, ctx: ctx, outbound: outbound,
		subscriptions: make(map[string]func()),
	}
	defer client.closeSubscriptions()
	for ctx.Err() == nil {
		var command Command
		if err := wsjson.Read(ctx, connection, &command); err != nil {
			break
		}
		select {
		case outbound <- client.handle(command):
		case <-ctx.Done():
		}
	}
	cancel()
	<-writerDone
	_ = connection.Close(websocket.StatusNormalClosure, "")
}

type serverClient struct {
	manager       *Manager
	config        *configuration.Store
	ctx           context.Context
	outbound      chan<- any
	mu            sync.Mutex
	subscriptions map[string]func()
}
