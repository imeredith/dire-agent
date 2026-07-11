import {
  Activity,
  Bot,
  CheckCircle2,
  CircleStop,
  Clock3,
  Eye,
  FileCode2,
  GitBranch,
  ListTree,
  MessageSquare,
  Radio,
  Send,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  TriangleAlert,
  Wrench,
} from "lucide-react";
import { useMemo, useState } from "react";
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Panel,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { activity, agents, changes, conversation, projects, steps } from "../mockData";
import "./flow-topology.css";

type AgentRecord = (typeof agents)[number];
type AgentFilter = "all" | "running" | "attention";

interface AgentNodeData extends Record<string, unknown> {
  agent: AgentRecord;
  dimmed: boolean;
  followLive: boolean;
}

type AgentNode = Node<AgentNodeData, "agent">;

const positions: Record<AgentRecord["id"], { x: number; y: number }> = {
  root: { x: 300, y: 20 },
  ux: { x: -10, y: 245 },
  ui: { x: 300, y: 245 },
  qa: { x: 610, y: 245 },
};

const nodeTypes = { agent: AgentNodeCard } as NodeTypes;

export function FlowTopology() {
  const [selectedID, setSelectedID] = useState<AgentRecord["id"]>("root");
  const [filter, setFilter] = useState<AgentFilter>("all");
  const [followLive, setFollowLive] = useState(true);
  const [inspectorTab, setInspectorTab] = useState<"transcript" | "activity" | "changes">("transcript");
  const [guidance, setGuidance] = useState("");
  const [sentGuidance, setSentGuidance] = useState("");

  const selectedAgent = agents.find((agent) => agent.id === selectedID) ?? agents[0];
  const matchesFilter = (agent: AgentRecord) => {
    if (filter === "running") return agent.status === "Running";
    if (filter === "attention") return agent.status === "Blocked";
    return true;
  };

  const nodes = useMemo<AgentNode[]>(() => agents.map((agent) => ({
    id: agent.id,
    type: "agent",
    position: positions[agent.id],
    draggable: false,
    connectable: false,
    selectable: true,
    selected: agent.id === selectedID,
    ariaLabel: `${agent.name}, ${agent.status}, ${agent.progress} percent complete. Select to inspect.`,
    data: {
      agent,
      dimmed: !matchesAgentFilter(agent, filter),
      followLive,
    },
  })), [filter, followLive, selectedID]);

  const edges = useMemo<Edge[]>(() => agents.slice(1).map((agent) => ({
    id: `root-${agent.id}`,
    source: "root",
    target: agent.id,
    type: "smoothstep",
    animated: followLive && agent.status === "Running",
    focusable: false,
    selectable: false,
    ariaLabel: `Delegated from orchestrator to ${agent.name}`,
    markerEnd: { type: MarkerType.ArrowClosed, color: edgeColor(agent.status), width: 15, height: 15 },
    style: {
      stroke: edgeColor(agent.status),
      strokeWidth: agent.status === "Running" ? 2.1 : 1.5,
      opacity: !matchesAgentFilter(agent, filter) ? 0.2 : 0.82,
    },
  })), [filter, followLive]);

  const submitGuidance = () => {
    const value = guidance.trim();
    if (!value) return;
    setSentGuidance(value);
    setGuidance("");
  };

  return (
    <div className="flow-topology">
      <header className="flow-topology__header">
        <div className="flow-topology__brand">
          <span><GitBranch aria-hidden="true" size={18} /></span>
          <div><strong>Dire Agent</strong><small>Live topology</small></div>
        </div>
        <div className="flow-topology__project-scope">
          <span>Project</span><strong>{projects[0].name}</strong><code>~/personal/dire-agent</code>
        </div>
        <div className="flow-topology__header-actions">
          <span className="flow-topology__read-only"><Eye aria-hidden="true" size={13} /> Observe only</span>
          <button
            className={followLive ? "flow-topology__follow active" : "flow-topology__follow"}
            type="button"
            aria-pressed={followLive}
            onClick={() => setFollowLive((value) => !value)}
          >
            <Radio aria-hidden="true" size={13} /> Follow live
          </button>
          <span className="flow-topology__online"><i aria-hidden="true" /> Online</span>
        </div>
      </header>

      <div className="flow-topology__workspace">
        <aside className="flow-topology__roster" aria-label="Agent roster">
          <section className="flow-topology__project-card">
            <header><span><TerminalSquare aria-hidden="true" size={15} /></span><div><strong>WebUI design space</strong><small>Root conversation</small></div></header>
            <div className="flow-topology__project-meta"><span><Activity size={12} /> 2 running</span><span><TriangleAlert size={12} /> 1 blocked</span></div>
          </section>

          <div className="flow-topology__section-heading"><span>Agents</span><em>{agents.length}</em></div>
          <div className="flow-topology__filters" aria-label="Filter agents">
            {(["all", "running", "attention"] as const).map((value) => (
              <button
                type="button"
                key={value}
                aria-pressed={filter === value}
                className={filter === value ? "active" : ""}
                onClick={() => setFilter(value)}
              >{value === "all" ? "All" : value === "running" ? "Live" : "Needs attention"}</button>
            ))}
          </div>

          <div className="flow-topology__agent-roster">
            {agents.filter(matchesFilter).map((agent) => (
              <button
                type="button"
                className={agent.id === selectedID ? "selected" : ""}
                onClick={() => setSelectedID(agent.id)}
                key={agent.id}
              >
                <span className={`flow-topology__status flow-topology__status--${statusSlug(agent.status)}`} aria-hidden="true" />
                <span><strong>{agent.name}</strong><small>{agent.role}</small></span>
                <em>{agent.progress}%</em>
              </button>
            ))}
            {!agents.some(matchesFilter) && <p className="flow-topology__empty">No agents match this filter.</p>}
          </div>

          <section className="flow-topology__plan-summary">
            <div className="flow-topology__section-heading"><span>Plan</span><em>3 / 4</em></div>
            <ol>
              {steps.map((step, index) => (
                <li className={`flow-topology__plan-step flow-topology__plan-step--${step.state}`} key={step.label}>
                  <span aria-hidden="true">{step.state === "done" ? <CheckCircle2 size={12} /> : index + 1}</span>
                  <p>{step.label}</p>
                </li>
              ))}
            </ol>
          </section>

          <footer><ShieldCheck aria-hidden="true" size={13} /><span><strong>Workspace sandbox</strong><small>Network denied</small></span></footer>
        </aside>

        <main className="flow-topology__map" aria-label="Live agent delegation map">
          <div className="flow-topology__map-heading">
            <div><span>DELEGATION MAP</span><h1>Parallel work, one glance.</h1></div>
            <p>Select an agent to inspect its transcript. Structure is controlled by the running conversation.</p>
          </div>
          <div className="flow-topology__canvas">
            <ReactFlow<AgentNode>
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              fitView
              fitViewOptions={{ padding: 0.24, minZoom: 0.62, maxZoom: 1.05 }}
              minZoom={0.45}
              maxZoom={1.55}
              nodesDraggable={false}
              nodesConnectable={false}
              elementsSelectable
              edgesFocusable={false}
              edgesReconnectable={false}
              deleteKeyCode={null}
              multiSelectionKeyCode={null}
              selectionKeyCode={null}
              panOnScroll
              selectionOnDrag={false}
              onNodeClick={(_, node) => setSelectedID(node.id as AgentRecord["id"])}
              onSelectionChange={({ nodes: selected }) => {
                if (selected[0]) setSelectedID(selected[0].id as AgentRecord["id"]);
              }}
              colorMode="dark"
              aria-label="Agent topology. Arrow keys move between selectable agents; the graph cannot be edited."
            >
              <Background variant={BackgroundVariant.Dots} gap={22} size={1.2} color="#2c3547" />
              <Controls showInteractive={false} position="bottom-left" aria-label="Map zoom controls" />
              <MiniMap
                ariaLabel="Agent topology overview"
                nodeColor={(node) => {
                  const agent = (node.data as AgentNodeData).agent;
                  return agent.status === "Running" ? "#56d7b4" : agent.status === "Blocked" ? "#f3ae56" : "#7f9cff";
                }}
                nodeStrokeColor="#0c1018"
                maskColor="rgba(7, 10, 16, .66)"
                pannable
                zoomable
                position="bottom-right"
              />
              <Panel position="top-left" className="flow-topology__canvas-note">
                <Eye aria-hidden="true" size={13} /> Read-only runtime view
              </Panel>
              <Panel position="top-right" className="flow-topology__legend" aria-label="Status legend">
                <span><i className="running" />Running</span><span><i className="done" />Done</span><span><i className="blocked" />Blocked</span>
              </Panel>
            </ReactFlow>
          </div>
          <div className="flow-topology__pulse-strip" aria-live={followLive ? "polite" : "off"}>
            <span><i aria-hidden="true" /> Live activity</span>
            <p><strong>ui-builder</strong> edited responsive shell</p>
            <time>10:42</time>
          </div>
        </main>

        <aside className="flow-topology__inspector" aria-label={`${selectedAgent.name} inspector`}>
          <header className="flow-topology__inspector-heading">
            <span className={`flow-topology__agent-avatar flow-topology__agent-avatar--${statusSlug(selectedAgent.status)}`}><Bot aria-hidden="true" size={18} /></span>
            <div><span>{selectedID === "root" ? "ROOT CONVERSATION" : "CHILD AGENT"}</span><h2>{selectedAgent.name}</h2><p>{selectedAgent.role}</p></div>
            <span className={`flow-topology__status-pill flow-topology__status-pill--${statusSlug(selectedAgent.status)}`}>{selectedAgent.status}</span>
          </header>

          <div className="flow-topology__progress-card" aria-label={`${selectedAgent.progress} percent complete`}>
            <div className="flow-topology__progress-ring" style={{ background: `conic-gradient(${statusColor(selectedAgent.status)} ${selectedAgent.progress}%, #263043 0)` }}><span>{selectedAgent.progress}%</span></div>
            <div><strong>{selectedAgent.status === "Blocked" ? "Waiting for preview" : selectedAgent.status === "Done" ? "Task complete" : "Task in progress"}</strong><small>{selectedAgent.status === "Blocked" ? "Dependency: ui-builder" : "Last event 18s ago"}</small></div>
          </div>

          <div className="flow-topology__inspector-tabs" role="tablist" aria-label="Agent details">
            {(["transcript", "activity", "changes"] as const).map((tab) => (
              <button
                type="button"
                role="tab"
                aria-selected={inspectorTab === tab}
                className={inspectorTab === tab ? "active" : ""}
                onClick={() => setInspectorTab(tab)}
                key={tab}
              >{tab === "transcript" ? "Transcript" : tab === "activity" ? "Activity" : "Changes"}</button>
            ))}
          </div>

          <div className="flow-topology__inspector-content">
            {inspectorTab === "transcript" && (
              <div className="flow-topology__transcript" role="tabpanel" aria-label="Transcript">
                {conversation.map((message, index) => (
                  <article className={message.role === "you" ? "outbound" : "inbound"} key={`${message.role}-${index}`}>
                    <small>{message.role === "you" ? "You" : selectedAgent.name}</small>
                    <p>{message.text}</p>
                  </article>
                ))}
                {selectedAgent.status === "Running" && <div className="flow-topology__tool-live"><Wrench size={13} /><span><strong>Applying changes</strong><small>2 files · 18s</small></span><i /></div>}
              </div>
            )}
            {inspectorTab === "activity" && (
              <div className="flow-topology__activity" role="tabpanel" aria-label="Agent activity">
                {activity.map((item) => (
                  <article key={`${item.time}-${item.agent}`}><time>{item.time}</time><span className={`flow-topology__status flow-topology__status--${item.tone}`} /><p><strong>{item.agent}</strong>{item.text}</p></article>
                ))}
                <article><time>10:31</time><span className="flow-topology__status flow-topology__status--done" /><p><strong>orchestrator</strong>Created the shared plan</p></article>
              </div>
            )}
            {inspectorTab === "changes" && (
              <div className="flow-topology__changes" role="tabpanel" aria-label="Changed files">
                {changes.map((change) => (
                  <button type="button" key={change.file}><FileCode2 size={14} /><span><strong>{change.file}</strong><small>{change.state}</small></span><em>{change.delta}</em></button>
                ))}
              </div>
            )}
          </div>

          <form className="flow-topology__guidance" onSubmit={(event) => { event.preventDefault(); submitGuidance(); }}>
            <label htmlFor="flow-topology-guidance">Guide {selectedAgent.name}</label>
            {sentGuidance && <p role="status"><CheckCircle2 size={12} /> Guidance queued</p>}
            <div>
              <textarea id="flow-topology-guidance" rows={2} value={guidance} onChange={(event) => setGuidance(event.target.value)} placeholder="Send a focused note…" />
              <button type="submit" disabled={!guidance.trim()} aria-label={`Send guidance to ${selectedAgent.name}`}><Send size={14} /></button>
            </div>
          </form>
        </aside>
      </div>
    </div>
  );
}

function AgentNodeCard({ data, selected }: NodeProps<AgentNode>) {
  const { agent, dimmed, followLive } = data;
  const slug = statusSlug(agent.status);
  return (
    <div
      className={`flow-topology__node flow-topology__node--${slug}${selected ? " selected" : ""}${dimmed ? " dimmed" : ""}${followLive && agent.status === "Running" ? " live" : ""}`}
    >
      <Handle type="target" position={Position.Top} isConnectable={false} className="flow-topology__handle" />
      <header>
        <span className={`flow-topology__node-avatar flow-topology__node-avatar--${slug}`}><Bot aria-hidden="true" size={15} /></span>
        <span><strong>{agent.name}</strong><small>{agent.id === "root" ? "Root conversation" : "Child agent"}</small></span>
        <i className={`flow-topology__node-signal flow-topology__node-signal--${slug}`} aria-hidden="true" />
      </header>
      <p>{agent.role}</p>
      <div className="flow-topology__node-progress"><span><i style={{ width: `${agent.progress}%` }} /></span><em>{agent.progress}%</em></div>
      <footer><span>{agent.status}</span><span>{agent.status === "Blocked" ? <><Clock3 size={11} /> waiting</> : agent.status === "Done" ? <><CheckCircle2 size={11} /> complete</> : <><Activity size={11} /> active</>}</span></footer>
      <Handle type="source" position={Position.Bottom} isConnectable={false} className="flow-topology__handle" />
    </div>
  );
}

function matchesAgentFilter(agent: AgentRecord, filter: AgentFilter) {
  if (filter === "running") return agent.status === "Running";
  if (filter === "attention") return agent.status === "Blocked";
  return true;
}

function statusSlug(status: AgentRecord["status"] | string) {
  return status.toLowerCase();
}

function statusColor(status: AgentRecord["status"]) {
  if (status === "Running") return "#56d7b4";
  if (status === "Blocked") return "#f3ae56";
  return "#7f9cff";
}

function edgeColor(status: AgentRecord["status"]) {
  if (status === "Running") return "#3baf98";
  if (status === "Blocked") return "#b57838";
  return "#627cc5";
}

export default FlowTopology;
