package agentteam_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
)

func TestToolsSpawnAndMessageWithCallerIdentity(t *testing.T) {
	backend := &fakeBackend{}
	tools := agentteam.Tools(backend, agentteam.Scope{
		AgentID: "project_root", CanSpawn: true,
		Profiles: map[string]string{"explore": "Read-only exploration."},
	})
	if len(tools) != 5 {
		t.Fatalf("tools = %v", tools)
	}
	output, err := tools["spawn_agent"].Execute(context.Background(), json.RawMessage(`{"name":"searcher","profile":"explore","task":"find the code"}`))
	if err != nil || backend.spawn.ParentID != "project_root" || backend.spawn.Name != "searcher" || !json.Valid([]byte(output)) {
		t.Fatalf("spawn = %q request=%+v err=%v", output, backend.spawn, err)
	}
	if _, err := tools["send_agent_message"].Execute(context.Background(), json.RawMessage(`{"agent_id":"agent_1","message":"status?"}`)); err != nil {
		t.Fatal(err)
	}
	if backend.from != "project_root" || backend.to != "agent_1" || !backend.wake {
		t.Fatalf("message routing = %s -> %s wake=%v", backend.from, backend.to, backend.wake)
	}
}

func TestChildWithoutSpawnPermissionCannotCreateGrandchildren(t *testing.T) {
	tools := agentteam.Tools(&fakeBackend{}, agentteam.Scope{AgentID: "agent_1", CanSpawn: false})
	if tools["spawn_agent"] != nil || tools["list_agents"] == nil || tools["send_agent_message"] == nil {
		t.Fatalf("tools = %v", tools)
	}
}

type fakeBackend struct {
	spawn agentteam.SpawnRequest
	from  string
	to    string
	wake  bool
}

func (f *fakeBackend) SpawnAgent(_ context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	f.spawn = request
	return agentteam.Agent{ID: "agent_1", ParentID: request.ParentID, Name: request.Name, Status: "running"}, nil
}
func (*fakeBackend) ListAgents(context.Context, string) ([]agentteam.Agent, error) {
	return []agentteam.Agent{{ID: "agent_1", Status: "idle"}}, nil
}
func (f *fakeBackend) SendAgentMessage(_ context.Context, from, to, _ string, wake bool) (agentteam.Message, error) {
	f.from, f.to, f.wake = from, to, wake
	return agentteam.Message{ID: "message_1", FromID: from, ToID: to}, nil
}
func (*fakeBackend) WaitAgents(context.Context, string, []string, time.Duration) (agentteam.WaitResult, error) {
	return agentteam.WaitResult{}, nil
}
func (*fakeBackend) InterruptAgent(context.Context, string, string) error { return nil }
