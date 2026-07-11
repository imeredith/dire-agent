import {
  Activity,
  Bot,
  Check,
  ChevronRight,
  CirclePause,
  CirclePlay,
  Clock3,
  Code2,
  Command,
  Eye,
  FileCode2,
  GitBranch,
  Layers3,
  MessageSquareText,
  MoreHorizontal,
  Search,
  Send,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  Zap,
} from "lucide-react";
import { FormEvent, useMemo, useState } from "react";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import { activity, agents, changes, conversation, projects, steps } from "../mockData";
import "./ShadcnCommandCenter.css";

const quickPrompts = ["Summarise blockers", "Review the diff", "Open preview"];

function statusTone(status: string) {
  if (status === "Running" || status === "running") return "live";
  if (status === "Done" || status === "done") return "done";
  if (status === "Blocked" || status === "review") return "blocked";
  return "idle";
}

export function ShadcnCommandCenter() {
  const [selectedProject, setSelectedProject] = useState<string>(projects[0].name);
  const [selectedAgent, setSelectedAgent] = useState<string>(agents[2].id);
  const [running, setRunning] = useState(true);
  const [composer, setComposer] = useState("");
  const [announcement, setAnnouncement] = useState("Three agents are working in parallel.");

  const activeAgent = useMemo(
    () => agents.find((agent) => agent.id === selectedAgent) ?? agents[0],
    [selectedAgent],
  );

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const message = composer.trim();
    if (!message) return;
    setAnnouncement(`Guidance queued for ${activeAgent.name}: ${message}`);
    setComposer("");
  };

  return (
    <section className="scc-shell" aria-label="Shadcn command center concept">
      <header className="scc-topbar">
        <div className="scc-brand" aria-label="Dire Agent">
          <span className="scc-brand-mark"><TerminalSquare size={17} /></span>
          <span><strong>dire</strong><small>command center</small></span>
        </div>

        <div className="scc-breadcrumb" aria-label="Current workspace">
          <span>Workspaces</span><ChevronRight size={13} />
          <strong>{selectedProject}</strong>
        </div>

        <div className="scc-top-actions">
          <Badge className="scc-health-badge"><span className="scc-live-dot" /> Daemon healthy</Badge>
          <Button variant="ghost" size="icon" aria-label="Search commands"><Search size={16} /></Button>
          <Button variant="outline" size="sm" onClick={() => setRunning((value) => !value)}>
            {running ? <CirclePause size={14} /> : <CirclePlay size={14} />}
            <span>{running ? "Pause run" : "Resume run"}</span>
          </Button>
        </div>
      </header>

      <div className="scc-layout">
        <aside className="scc-project-rail" aria-label="Projects">
          <div className="scc-rail-heading">
            <span><Layers3 size={13} /> Projects</span>
            <Button variant="ghost" size="icon" aria-label="Project options"><MoreHorizontal size={15} /></Button>
          </div>
          <div className="scc-project-list">
            {projects.map((project, index) => (
              <button
                type="button"
                key={project.name}
                className={`scc-project ${selectedProject === project.name ? "selected" : ""}`}
                onClick={() => {
                  setSelectedProject(project.name);
                  setAnnouncement(`${project.name} selected.`);
                }}
                aria-pressed={selectedProject === project.name}
              >
                <span className="scc-project-index">{String(index + 1).padStart(2, "0")}</span>
                <span className="scc-project-copy"><strong>{project.name}</strong><small>{project.meta}</small></span>
                <span className={`scc-project-state ${statusTone(project.status)}`} aria-label={project.status} />
              </button>
            ))}
          </div>

          <div className="scc-safety-card">
            <div><ShieldCheck size={15} /><Badge variant="outline">Workspace</Badge></div>
            <strong>Tools stay in scope</strong>
            <p>4 folders · network off</p>
            <button type="button" onClick={() => setAnnouncement("Sandbox details opened.")}>Inspect sandbox <ChevronRight size={12} /></button>
          </div>
        </aside>

        <main className="scc-main">
          <section className="scc-plan-card" aria-labelledby="scc-plan-title">
            <div className="scc-plan-overview">
              <span className="scc-kicker"><Sparkles size={12} /> Active mission</span>
              <div className="scc-plan-title-row">
                <div>
                  <h1 id="scc-plan-title">Explore the WebUI design space</h1>
                  <p>Ten distinct concepts, compared against proven agent-workspace patterns.</p>
                </div>
                <Badge variant="secondary" className="scc-context-badge">68% context</Badge>
              </div>
              <div className="scc-progress-track" role="progressbar" aria-label="Mission progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={68}>
                <span style={{ width: "68%" }} />
              </div>
            </div>
            <ol className="scc-plan-steps">
              {steps.map((step, index) => (
                <li key={step.label} className={step.state}>
                  <span>{step.state === "done" ? <Check size={12} /> : index + 1}</span>
                  <div><strong>{step.label}</strong><small>{step.state === "active" ? "ui-builder is working" : step.state}</small></div>
                </li>
              ))}
            </ol>
          </section>

          <Tabs defaultValue="work" className="scc-work-tabs">
            <div className="scc-tabs-bar">
              <TabsList className="scc-tabs-list" aria-label="Project surfaces">
                <TabsTrigger value="work"><MessageSquareText size={13} /> Work</TabsTrigger>
                <TabsTrigger value="changes"><GitBranch size={13} /> Changes <span className="scc-tab-count">3</span></TabsTrigger>
                <TabsTrigger value="preview"><Eye size={13} /> Preview</TabsTrigger>
              </TabsList>
              <div className="scc-command-hint"><Command size={12} /> K <span>Command menu</span></div>
            </div>

            <TabsContent value="work" className="scc-tab-panel scc-work-panel">
              <div className="scc-transcript" aria-label="Conversation transcript">
                <div className="scc-time-divider"><span>Today · 10:31</span></div>
                {conversation.map((message) => (
                  <article key={`${message.role}-${message.text}`} className={`scc-message ${message.role}`}>
                    <span className="scc-message-avatar">{message.role === "you" ? "IV" : <Bot size={14} />}</span>
                    <div>
                      <header><strong>{message.role === "you" ? "You" : "Dire Agent"}</strong><small>{message.role === "you" ? "10:31" : "10:32"}</small></header>
                      <p>{message.text}</p>
                    </div>
                  </article>
                ))}
                <article className="scc-live-tool">
                  <span><Code2 size={14} /></span>
                  <div><strong>ui-builder is editing the design lab</strong><small>2 files changed · 38 seconds</small></div>
                  <Activity size={14} />
                </article>
              </div>

              <form className="scc-composer" onSubmit={submit}>
                <div className="scc-quick-prompts" aria-label="Suggested prompts">
                  {quickPrompts.map((prompt) => (
                    <button type="button" key={prompt} onClick={() => setComposer(prompt)}>{prompt}</button>
                  ))}
                </div>
                <label>
                  <span className="sr-only">Send guidance to the selected agent</span>
                  <textarea
                    rows={2}
                    value={composer}
                    onChange={(event) => setComposer(event.target.value)}
                    placeholder={`Guide ${activeAgent.name}, ask a question, or type / for commands…`}
                  />
                </label>
                <div className="scc-composer-footer">
                  <div><Badge variant="secondary">gpt-5.6</Badge><Badge variant="outline">thinking: high</Badge></div>
                  <span>⌘ ↵ to send</span>
                  <Button size="sm" type="submit" disabled={!composer.trim()}><Send size={13} /> Send</Button>
                </div>
              </form>
            </TabsContent>

            <TabsContent value="changes" className="scc-tab-panel">
              <section className="scc-changes" aria-label="Changed files">
                <header><div><span className="scc-kicker"><GitBranch size={12} /> Review surface</span><h2>Three focused changes</h2></div><Button size="sm"><Check size={13} /> Mark reviewed</Button></header>
                {changes.map((change) => (
                  <button type="button" key={change.file} onClick={() => setAnnouncement(`${change.file} opened in the diff viewer.`)}>
                    <span className="scc-file-icon"><FileCode2 size={15} /></span>
                    <span><strong>{change.file}</strong><small>{change.state} by ui-builder</small></span>
                    <code>{change.delta}</code><ChevronRight size={14} />
                  </button>
                ))}
              </section>
            </TabsContent>

            <TabsContent value="preview" className="scc-tab-panel">
              <section className="scc-preview" aria-label="Responsive preview">
                <header><div><span className="scc-kicker"><Eye size={12} /> Live preview</span><h2>Parallel-agent workspace</h2></div><Badge>Synced 4s ago</Badge></header>
                <div className="scc-browser-frame">
                  <div className="scc-browser-bar"><i /><i /><i /><span>localhost:5173/design-lab</span></div>
                  <div className="scc-browser-page">
                    <aside><span /><span /><span /><span /></aside>
                    <main><div /><div className="wide" /><div /><div /></main>
                    <section><span /><span /><span /></section>
                  </div>
                </div>
              </section>
            </TabsContent>
          </Tabs>
        </main>

        <aside className="scc-agent-rail" aria-label="Agent activity">
          <div className="scc-agent-heading">
            <div><span className="scc-kicker"><Zap size={12} /> Live team</span><strong>3 working · 1 waiting</strong></div>
            <Button variant="ghost" size="icon" aria-label="Agent panel options"><MoreHorizontal size={15} /></Button>
          </div>

          <div className="scc-agent-list">
            {agents.map((agent) => (
              <button
                type="button"
                key={agent.id}
                className={selectedAgent === agent.id ? "selected" : ""}
                onClick={() => setSelectedAgent(agent.id)}
                aria-pressed={selectedAgent === agent.id}
              >
                <span className={`scc-agent-avatar ${statusTone(agent.status)}`}>{agent.name.slice(0, 2).toUpperCase()}</span>
                <span><strong>{agent.name}</strong><small>{agent.role}</small><i><b style={{ width: `${agent.progress}%` }} /></i></span>
                <Badge variant="outline" className={statusTone(agent.status)}>{agent.status}</Badge>
              </button>
            ))}
          </div>

          <section className="scc-agent-focus" aria-live="polite">
            <header><span><Bot size={14} /></span><div><strong>{activeAgent.name}</strong><small>{activeAgent.role}</small></div></header>
            <dl><div><dt>Progress</dt><dd>{activeAgent.progress}%</dd></div><div><dt>Last update</dt><dd>38s ago</dd></div></dl>
            <Button variant="outline" size="sm" onClick={() => setComposer(`Please report your next checkpoint, ${activeAgent.name}.`)}>Send guidance</Button>
          </section>

          <section className="scc-activity">
            <header><Clock3 size={13} /> Recent activity</header>
            {activity.map((item) => (
              <div key={`${item.time}-${item.agent}`}><time>{item.time}</time><span className={item.tone} /><p><strong>{item.agent}</strong>{item.text}</p></div>
            ))}
          </section>
        </aside>
      </div>
      <p className="scc-announcement" role="status">{announcement}</p>
    </section>
  );
}

export default ShadcnCommandCenter;
