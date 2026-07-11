package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imeredith/dire-agent/agent"
)

func TestStepEncodesImageInputForResponsesAPI(t *testing.T) {
	t.Parallel()
	authFile := writeTestAuth(t, "access-token", "refresh-token", "account-123", "plus", time.Now().Add(time.Hour))
	wantImage := []byte("image bytes")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if len(body.Input) != 1 {
			t.Errorf("input count = %d", len(body.Input))
		}
		var message struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL string `json:"image_url"`
				Detail   string `json:"detail"`
			} `json:"content"`
		}
		if len(body.Input) == 1 {
			_ = json.Unmarshal(body.Input[0], &message)
		}
		if len(message.Content) != 2 || message.Content[0].Type != "input_text" || message.Content[0].Text != "inspect" {
			t.Errorf("message content = %#v", message.Content)
		}
		wantURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(wantImage)
		if len(message.Content) < 2 || message.Content[1].Type != "input_image" || message.Content[1].ImageURL != wantURL || message.Content[1].Detail != "auto" {
			t.Errorf("image content = %#v, want URL prefix %q", message.Content, wantURL)
		}
		writeSSE(t, writer,
			map[string]any{"type": "response.output_item.done", "item": map[string]any{
				"id": "message-image", "type": "message", "role": "assistant", "phase": "final_answer",
				"content": []map[string]string{{"type": "output_text", "text": "seen"}},
			}},
			map[string]any{"type": "response.completed", "response": map[string]string{"id": "response-image"}},
		)
	}))
	defer server.Close()
	provider := newTestProvider(t, authFile, server)
	opened, err := provider.OpenSession(context.Background(), agent.SessionOptions{Model: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	session := opened.(agent.StepSession)
	result, err := session.Step(context.Background(), agent.StepRequest{
		UserMessages: []string{"inspect"},
		Images:       []agent.ImageInput{{Name: "paste.png", MimeType: "image/png", Data: wantImage}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Text) != "seen" {
		t.Fatalf("result = %#v", result)
	}
}

func TestCleanReasoningSummaryRemovesProviderCommentMarkers(t *testing.T) {
	got := cleanReasoningSummary("**Inspecting**\n\n<!-- -->")
	if got != "**Inspecting**" {
		t.Fatalf("cleanReasoningSummary() = %q", got)
	}
}
