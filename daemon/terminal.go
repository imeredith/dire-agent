package daemon

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/creack/pty"
)

const (
	TerminalModeShell   = "shell"
	TerminalModeLazygit = "lazygit"
	TerminalModeNvim    = "nvim"
)

type terminalClientMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

type terminalServerMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Mode    string `json:"mode,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *Server) handleTerminal(writer http.ResponseWriter, request *http.Request) {
	if s.Manager == nil {
		http.Error(writer, "daemon is not initialized", http.StatusServiceUnavailable)
		return
	}
	projectID := strings.TrimSpace(request.URL.Query().Get("project_id"))
	project, err := s.Manager.Project(request.Context(), projectID)
	if err != nil || project.IsSubagent() {
		http.Error(writer, "terminal requires a top-level project", http.StatusNotFound)
		return
	}
	launcherID := strings.TrimSpace(request.URL.Query().Get("launcher_id"))
	if launcherID == "" {
		// mode remains accepted for older clients; the built-in mode names are
		// also the IDs of their default configured launchers.
		launcherID = strings.ToLower(strings.TrimSpace(request.URL.Query().Get("mode")))
	}
	if launcherID == "" {
		launcherID = TerminalModeShell
	}
	_, launcher, err := configuredProjectLauncher(request.Context(), s.Manager, s.Config, project.ID, launcherID)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	command, err := projectTerminalCommand(request.Context(), project, launcher)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(1 << 20)

	cols, rows := terminalSize(request)
	pseudoterminal, err := pty.StartWithSize(command, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		_ = wsjson.Write(request.Context(), connection, terminalServerMessage{Type: "error", Message: err.Error()})
		_ = connection.Close(websocket.StatusInternalError, "terminal start failed")
		return
	}
	defer pseudoterminal.Close()
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
	}()

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	var writeMu sync.Mutex
	write := func(message terminalServerMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		writeContext, writeCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer writeCancel()
		return wsjson.Write(writeContext, connection, message)
	}
	if err := write(terminalServerMessage{Type: "ready", Mode: launcher.launcher.ID, CWD: project.CWD}); err != nil {
		return
	}

	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		buffer := make([]byte, 32<<10)
		for {
			count, readErr := pseudoterminal.Read(buffer)
			if count > 0 {
				if write(terminalServerMessage{
					Type: "output", Data: base64.StdEncoding.EncodeToString(buffer[:count]),
				}) != nil {
					cancel()
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					_ = write(terminalServerMessage{Type: "error", Message: readErr.Error()})
				}
				return
			}
		}
	}()

	processDone := make(chan struct{})
	go func() {
		err := command.Wait()
		code := 0
		message := ""
		if err != nil {
			message = err.Error()
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				code = exitError.ExitCode()
			} else {
				code = -1
			}
		}
		<-outputDone
		_ = write(terminalServerMessage{Type: "exit", Code: code, Message: message})
		close(processDone)
		_ = connection.Close(websocket.StatusNormalClosure, "terminal exited")
	}()

	for ctx.Err() == nil {
		var message terminalClientMessage
		if err := wsjson.Read(ctx, connection, &message); err != nil {
			break
		}
		switch message.Type {
		case "input":
			if len(message.Data) > 64<<10 {
				_ = write(terminalServerMessage{Type: "error", Message: "terminal input frame is too large"})
				continue
			}
			if _, err := pseudoterminal.Write([]byte(message.Data)); err != nil {
				_ = write(terminalServerMessage{Type: "error", Message: err.Error()})
			}
		case "resize":
			if message.Cols < 2 || message.Rows < 1 || message.Cols > 1000 || message.Rows > 1000 {
				continue
			}
			if err := pty.Setsize(pseudoterminal, &pty.Winsize{Cols: message.Cols, Rows: message.Rows}); err != nil {
				_ = write(terminalServerMessage{Type: "error", Message: err.Error()})
			}
		}
	}
	cancel()
	_ = pseudoterminal.Close()
	select {
	case <-processDone:
	case <-time.After(2 * time.Second):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
	}
}

func terminalEnvironment(base []string, projectID string) []string {
	// The daemon may itself run in a non-interactive environment with TERM=dumb
	// or NO_COLOR set. Neither should leak into an explicitly interactive PTY.
	replaced := map[string]struct{}{
		"CLICOLOR":              {},
		"CLICOLOR_FORCE":        {},
		"COLORTERM":             {},
		"COLORFGBG":             {},
		"FORCE_COLOR":           {},
		"GOAGENT_PROJECT_ID":    {},
		"NO_COLOR":              {},
		"TERM":                  {},
		"TERM_PROGRAM":          {},
		"DIRE_AGENT_PROJECT_ID": {},
	}
	environment := make([]string, 0, len(base)+6)
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, skip := replaced[strings.ToUpper(name)]; skip {
				continue
			}
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"TERM_PROGRAM=dire-agent",
		"COLORFGBG=15;0",
		"CLICOLOR=1",
		"DIRE_AGENT_PROJECT_ID="+projectID,
		"GOAGENT_PROJECT_ID="+projectID,
	)
}

func terminalSize(request *http.Request) (uint16, uint16) {
	parse := func(name string, fallback uint16) uint16 {
		value, err := strconv.ParseUint(request.URL.Query().Get(name), 10, 16)
		if err != nil || value == 0 || value > 1000 {
			return fallback
		}
		return uint16(value)
	}
	return parse("cols", 120), parse("rows", 36)
}
