package daemon_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestPastedImageIsSandboxedPersistedAndSentToModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &imageCaptureProvider{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root, DefaultModel: "image-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Model: "image-model"})
	if err != nil {
		t.Fatal(err)
	}
	imageBytes := []byte("not-a-real-png-but-provider-bytes-are-preserved")
	err = manager.PromptWithAttachments(ctx, project.ID, "inspect this", "", []daemon.ImageAttachment{{
		Name: "clipboard.png", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(imageBytes),
	}})
	if err != nil {
		t.Fatal(err)
	}
	for {
		state, err := manager.State(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	provider.mu.Lock()
	images := append([]agent.ImageInput(nil), provider.images...)
	provider.mu.Unlock()
	if len(images) != 1 || images[0].MimeType != "image/png" || string(images[0].Data) != string(imageBytes) {
		t.Fatalf("model images = %#v", images)
	}
	messages, err := manager.Messages(ctx, project.ID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var stored daemon.ImageAttachment
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		var data struct {
			Attachments []daemon.ImageAttachment `json:"attachments"`
		}
		if json.Unmarshal(message.Data, &data) == nil && len(data.Attachments) == 1 {
			stored = data.Attachments[0]
		}
	}
	if stored.File == "" || stored.Data != "" || strings.Contains(stored.File, "clipboard") {
		t.Fatalf("stored attachment = %#v", stored)
	}
	path := filepath.Join(root, ".dire-agent", "attachments", stored.File)
	if contents, err := os.ReadFile(path); err != nil || string(contents) != string(imageBytes) {
		t.Fatalf("sandbox attachment = %q, %v", contents, err)
	}

	request := httptest.NewRequest("GET", "/attachments/"+project.ID+"/"+stored.File, nil)
	recorder := httptest.NewRecorder()
	(&daemon.Server{Manager: manager}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != 200 || recorder.Body.String() != string(imageBytes) || recorder.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("attachment response = %d %q %q", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}

	legacyDirectory := filepath.Join(root, ".goagent", "attachments")
	if err := os.MkdirAll(legacyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(legacyDirectory, stored.File)); err != nil {
		t.Fatal(err)
	}
	legacyRecorder := httptest.NewRecorder()
	(&daemon.Server{Manager: manager}).Handler().ServeHTTP(legacyRecorder, request)
	if legacyRecorder.Code != 200 || legacyRecorder.Body.String() != string(imageBytes) {
		t.Fatalf("legacy attachment response = %d %q", legacyRecorder.Code, legacyRecorder.Body.String())
	}

	outside := t.TempDir()
	outsideAttachments := filepath.Join(outside, "attachments")
	if err := os.MkdirAll(outsideAttachments, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideAttachments, stored.File), imageBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".goagent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".goagent")); err != nil {
		t.Fatal(err)
	}
	escapeRecorder := httptest.NewRecorder()
	(&daemon.Server{Manager: manager}).Handler().ServeHTTP(escapeRecorder, request)
	if escapeRecorder.Code != 404 {
		t.Fatalf("legacy attachment symlink escape response = %d %q", escapeRecorder.Code, escapeRecorder.Body.String())
	}
}

func TestPastedImageRejectsChatAndSymlinkEscape(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{Store: store, Provider: &imageCaptureProvider{}, DefaultCWD: root})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	attachment := []daemon.ImageAttachment{{Name: "x.png", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("x"))}}
	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.PromptWithAttachments(ctx, chat.ID, "look", "", attachment); err == nil {
		t.Fatal("pathless chat accepted an image attachment")
	}
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	escape := t.TempDir()
	if err := os.Symlink(escape, filepath.Join(root, ".dire-agent")); err != nil {
		t.Fatal(err)
	}
	if err := manager.PromptWithAttachments(ctx, project.ID, "look", "", attachment); err == nil {
		t.Fatal("image upload followed a sandbox-escaping .dire-agent symlink")
	}
	if _, err := os.Stat(filepath.Join(escape, "attachments")); !os.IsNotExist(err) {
		t.Fatalf("upload wrote outside sandbox: %v", err)
	}
}

type imageCaptureProvider struct {
	mu     sync.Mutex
	images []agent.ImageInput
}

func (p *imageCaptureProvider) OpenSession(context.Context, agent.SessionOptions) (agent.Session, error) {
	return &imageCaptureSession{provider: p, id: "image-session"}, nil
}
func (p *imageCaptureProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	return &imageCaptureSession{provider: p, id: state.ID}, nil
}
func (p *imageCaptureProvider) Close() error { return nil }

type imageCaptureSession struct {
	provider *imageCaptureProvider
	id       string
}

func (s *imageCaptureSession) ID() string { return s.id }
func (s *imageCaptureSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}
func (s *imageCaptureSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.provider.mu.Lock()
	s.provider.images = append([]agent.ImageInput(nil), request.Images...)
	s.provider.mu.Unlock()
	return agent.StepResult{Result: agent.Result{Text: "image received", SessionID: s.id}}, nil
}
func (s *imageCaptureSession) State() (agent.SessionState, error) {
	return agent.SessionState{ID: s.id, Provider: "image", Data: json.RawMessage(`[]`)}, nil
}
