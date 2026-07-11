import {
  Activity,
  ArrowRight,
  Bell,
  Blocks,
  Bot,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleDot,
  Clock3,
  Code2,
  Columns3,
  Command,
  FileCode2,
  FileText,
  Folder,
  GitBranch,
  Home,
  Layers3,
  ListChecks,
  MessageCircle,
  MoreHorizontal,
  Paperclip,
  Play,
  Plus,
  Radio,
  RotateCcw,
  Search,
  Send,
  Settings,
  ShieldCheck,
  Smartphone,
  Sparkles,
  SquareTerminal,
  TerminalSquare,
  UserRound,
} from "lucide-react";
import { useState } from "react";
import { activity, agents, changes, conversation, projects, steps } from "../mockData";
import "./custom-concepts.css";

export function AttentionInbox() {
  const [selected, setSelected] = useState("qa");
  const selectedAgent = agents.find((agent) => agent.id === selected) ?? agents[0];

  return (
    <div className="attention-concept">
      <aside className="attention-rail" aria-label="Primary navigation">
        <div className="attention-logo"><Command size={20} /></div>
        <button className="is-active" aria-label="Home"><Home size={19} /></button>
        <button aria-label="Activity"><Activity size={19} /></button>
        <button aria-label="Projects"><Layers3 size={19} /></button>
        <button aria-label="Search"><Search size={19} /></button>
        <span />
        <button aria-label="Settings"><Settings size={19} /></button>
        <div className="attention-avatar">IM</div>
      </aside>

      <main className="attention-main">
        <header className="attention-header">
          <div>
            <span className="attention-kicker">Sunday, 12 July</span>
            <h2>Good morning, Ivan</h2>
            <p>One agent needs you. Two runs are moving on their own.</p>
          </div>
          <button className="attention-new"><Plus size={16} /> New task</button>
        </header>

        <section className="attention-summary" aria-label="Run summary">
          <button className="is-selected"><Radio size={17} /><span><strong>2</strong>Running</span></button>
          <button><Bell size={17} /><span><strong>1</strong>Needs you</span></button>
          <button><GitBranch size={17} /><span><strong>3</strong>Ready to review</span></button>
          <button><CheckCircle2 size={17} /><span><strong>12</strong>Done this week</span></button>
        </section>

        <div className="attention-board">
          <section className="attention-column">
            <header><span><i className="attention-dot running" /> In progress</span><small>2</small></header>
            {agents.slice(0, 2).map((agent, index) => (
              <button className={`attention-card ${selected === agent.id ? "selected" : ""}`} onClick={() => setSelected(agent.id)} key={agent.id}>
                <span className="attention-card-project">{index === 0 ? "DIRE AGENT WEBUI" : "CHECKOUT RELIABILITY"}</span>
                <strong>{index === 0 ? "Build ten interface directions" : "Harden webhook retry flow"}</strong>
                <p>{agent.role}</p>
                <div className="attention-progress"><i style={{ width: `${agent.progress}%` }} /></div>
                <footer><span><Bot size={14} /> {agent.name}</span><span>{agent.progress}%</span></footer>
              </button>
            ))}
          </section>

          <section className="attention-column attention-needs">
            <header><span><i className="attention-dot needs" /> Needs you</span><small>1</small></header>
            <button className={`attention-card needs ${selected === "qa" ? "selected" : ""}`} onClick={() => setSelected("qa")}>
              <span className="attention-card-project">DIRE AGENT WEBUI</span>
              <strong>Preview build needs a decision</strong>
              <p>Use a task-first URL structure or keep one switchable gallery?</p>
              <div className="attention-question"><MessageCircle size={14} /> Asked 8 minutes ago</div>
              <footer><span><Bot size={14} /> qa</span><span className="attention-urgent">Reply</span></footer>
            </button>
          </section>

          <section className="attention-column">
            <header><span><i className="attention-dot review" /> Review</span><small>3</small></header>
            <button className="attention-card review" onClick={() => setSelected("ux")}>
              <span className="attention-card-project">CHECKOUT RELIABILITY</span>
              <strong>Idempotency guard is ready</strong>
              <p>5 files changed · 14 tests passing</p>
              <div className="attention-diff"><span>+128</span><span>−34</span></div>
              <footer><span><GitBranch size={14} /> checkpoint 7</span><ArrowRight size={15} /></footer>
            </button>
            <button className="attention-card muted" onClick={() => setSelected("ux")}>
              <span className="attention-card-project">DOCS REFRESH</span>
              <strong>New installation guide</strong>
              <p>Waiting for your editorial review</p>
              <footer><span><FileText size={14} /> 2 artifacts</span><ArrowRight size={15} /></footer>
            </button>
          </section>
        </div>
      </main>

      <aside className="attention-detail" aria-label="Selected run details">
        <header><span className={`attention-status ${selectedAgent.status.toLowerCase()}`}>{selectedAgent.status}</span><button aria-label="More actions"><MoreHorizontal size={18} /></button></header>
        <div className="attention-detail-icon"><Bot size={21} /></div>
        <h3>{selectedAgent.name}</h3>
        <p>{selectedAgent.role}</p>
        <div className="attention-detail-project"><Folder size={14} /> Dire Agent WebUI <ChevronRight size={13} /> Design lab</div>
        <section>
          <h4>Current step</h4>
          <strong>{selectedAgent.id === "qa" ? "Waiting for your decision" : "Building the comparison surface"}</strong>
          <p>{selectedAgent.id === "qa" ? "The implementation can support both, but the choice changes how reviewers navigate between concepts." : "Keeping the app connected to fixture data while preserving the live daemon route."}</p>
        </section>
        <section>
          <h4>Recent activity</h4>
          {activity.slice(0, 2).map((item) => <div className="attention-event" key={item.time}><i /><span><strong>{item.text}</strong><small>{item.time} · {item.agent}</small></span></div>)}
        </section>
        <div className="attention-reply">
          <textarea aria-label="Reply to agent" placeholder="Reply or steer this run…" rows={3} />
          <button aria-label="Send reply"><Send size={15} /></button>
        </div>
      </aside>
    </div>
  );
}

export function TerminalIDE() {
  const [activeTab, setActiveTab] = useState("terminal");

  return (
    <div className="terminal-concept">
      <aside className="terminal-activity" aria-label="IDE navigation">
        <div className="terminal-mark"><TerminalSquare size={23} /></div>
        <button className="active" aria-label="Explorer"><FileCode2 size={21} /></button>
        <button aria-label="Search"><Search size={21} /></button>
        <button aria-label="Source control"><GitBranch size={21} /><i>3</i></button>
        <button aria-label="Agents"><Bot size={21} /><i>2</i></button>
        <button aria-label="Extensions"><Blocks size={21} /></button>
        <span />
        <button aria-label="Account"><UserRound size={21} /></button>
        <button aria-label="Settings"><Settings size={21} /></button>
      </aside>

      <aside className="terminal-explorer">
        <header><span>EXPLORER</span><MoreHorizontal size={16} /></header>
        <div className="terminal-project"><ChevronRight size={13} className="rotated" /><strong>DIRE-AGENT</strong></div>
        <div className="terminal-tree">
          <span><ChevronRight size={12} className="rotated" /><Folder size={14} /> web</span>
          <span className="indent"><ChevronRight size={12} className="rotated" /><Folder size={14} /> src</span>
          <span className="indent2 selected"><FileCode2 size={14} /> DesignLabApp.tsx <i>M</i></span>
          <span className="indent2"><FileCode2 size={14} /> main.tsx <i>M</i></span>
          <span><ChevronRight size={12} /><Folder size={14} /> daemon</span>
          <span><FileText size={14} /> README.md</span>
        </div>
        <section className="terminal-agents">
          <header><span>ACTIVE AGENTS</span><Plus size={14} /></header>
          {agents.slice(0, 3).map((agent) => (
            <button key={agent.id}><i className={agent.status.toLowerCase()} /><span><strong>{agent.name}</strong><small>{agent.role}</small></span></button>
          ))}
        </section>
      </aside>

      <main className="terminal-workbench">
        <div className="terminal-tabs" role="tablist">
          {[
            ["editor", "DesignLabApp.tsx", <Code2 size={14} />],
            ["terminal", "Terminal", <SquareTerminal size={14} />],
            ["changes", "Changes · 3", <GitBranch size={14} />],
            ["preview", "Preview", <Play size={14} />],
          ].map(([id, label, icon]) => (
            <button key={String(id)} role="tab" aria-selected={activeTab === id} className={activeTab === id ? "active" : ""} onClick={() => setActiveTab(String(id))}>{icon}{label}</button>
          ))}
        </div>
        <div className="terminal-breadcrumbs"><span>dire-agent</span><ChevronRight size={12} /><span>web</span><ChevronRight size={12} /><span>src</span><ChevronRight size={12} /><strong>{activeTab === "editor" ? "DesignLabApp.tsx" : String(activeTab)}</strong></div>

        {activeTab === "terminal" && (
          <div className="terminal-screen" role="tabpanel" aria-label="Terminal output">
            <div className="terminal-screen-top"><span>zsh</span><span><Plus size={13} /><Columns3 size={13} /><MoreHorizontal size={14} /></span></div>
            <pre><span className="prompt">~/personal/dire-agent/web</span> <span className="branch">codex/design-lab*</span>{"\n"}<strong>❯</strong> npm run build{"\n\n"}<span className="muted">&gt; dire-agent-web@0.1.0 build</span>{"\n"}<span className="muted">&gt; tsc -b &amp;&amp; vite build</span>{"\n\n"}<span className="green">✓</span> 2,142 modules transformed.{"\n"}<span className="green">✓</span> built in 3.84s{"\n\n"}<strong>❯</strong> <span className="cursor"> </span></pre>
          </div>
        )}
        {activeTab === "editor" && (
          <div className="terminal-editor" role="tabpanel" aria-label="Code editor">
            <ol>
              <li><span className="pink">import</span> {'{'} <span className="blue">useState</span> {'}'} <span className="pink">from</span> <span className="orange">&quot;react&quot;</span>;</li>
              <li />
              <li><span className="pink">export function</span> <span className="yellow">DesignLabApp</span>() {'{'}</li>
              <li>&nbsp;&nbsp;<span className="pink">const</span> [concept, setConcept] = <span className="yellow">useState</span>(<span className="orange">&quot;command-center&quot;</span>);</li>
              <li />
              <li>&nbsp;&nbsp;<span className="pink">return</span> (</li>
              <li>&nbsp;&nbsp;&nbsp;&nbsp;&lt;<span className="blue">DesignGallery</span> concept={'{'}concept{'}'} /&gt;</li>
              <li>&nbsp;&nbsp;);</li>
              <li>{'}'}</li>
            </ol>
          </div>
        )}
        {activeTab === "changes" && (
          <div className="terminal-changes" role="tabpanel" aria-label="Changed files">
            <header><span>3 changed files</span><button><Check size={14} /> Review complete</button></header>
            {changes.map((change) => <div key={change.file}><FileCode2 size={15} /><span><strong>{change.file}</strong><small>Modified in this checkpoint</small></span><i>{change.delta}</i></div>)}
          </div>
        )}
        {activeTab === "preview" && (
          <div className="terminal-preview" role="tabpanel" aria-label="Application preview"><div><Sparkles size={30} /><strong>Preview is ready</strong><span>Open the design gallery at localhost:5173/designs</span><button><Play size={14} /> Open preview</button></div></div>
        )}
      </main>

      <aside className="terminal-chat">
        <header><div><Bot size={17} /><span><strong>orchestrator</strong><small><i /> working · 68%</small></span></div><MoreHorizontal size={16} /></header>
        <div className="terminal-plan">
          <div><span>Plan</span><strong>3 of 4</strong></div>
          {steps.map((step) => <span key={step.label} className={step.state}><i>{step.state === "done" ? <Check size={10} /> : ""}</i>{step.label}</span>)}
        </div>
        <div className="terminal-chat-stream">
          {conversation.map((message) => <article key={message.role} className={message.role}><small>{message.role === "you" ? "YOU" : "ORCHESTRATOR"}</small><p>{message.text}</p></article>)}
          <article className="tool"><div><SquareTerminal size={14} /><span><strong>Ran build</strong><small>npm run build</small></span><CheckCircle2 size={15} /></div></article>
        </div>
        <div className="terminal-composer">
          <textarea aria-label="Message orchestrator" rows={3} placeholder="Ask, steer, or queue a follow-up…" />
          <footer><span><button aria-label="Attach file"><Paperclip size={15} /></button><button className="mode">Autonomous <ChevronRight size={12} /></button></span><button className="send" aria-label="Send"><Send size={15} /></button></footer>
        </div>
        <div className="terminal-context"><span>gpt-5.6 · high</span><span>42k / 372k</span></div>
      </aside>

      <footer className="terminal-status"><span><GitBranch size={12} /> codex/design-lab*</span><span><RotateCcw size={12} /> checkpoint 8</span><i /><span><Radio size={12} /> daemon online</span><span>UTF-8</span><span>Ln 42, Col 18</span></footer>
    </div>
  );
}

export function NotebookWorkspace() {
  const [view, setView] = useState("journal");

  return (
    <div className="notebook-concept">
      <aside className="notebook-sidebar">
        <div className="notebook-brand"><div>D</div><strong>Dire</strong></div>
        <button className="notebook-search"><Search size={15} /> Search <kbd>⌘K</kbd></button>
        <nav>
          <button className="active"><FileText size={16} /> Work journal</button>
          <button><ListChecks size={16} /> Tasks <span>4</span></button>
          <button><Bot size={16} /> Agents <span className="live">2</span></button>
          <button><GitBranch size={16} /> Changes <span>3</span></button>
        </nav>
        <section>
          <header><span>PROJECTS</span><Plus size={14} /></header>
          {projects.slice(0, 3).map((project, index) => <button key={project.name} className={index === 0 ? "selected" : ""}><span className={`notebook-project-dot ${project.status}`} /> <span><strong>{project.name}</strong><small>{project.meta}</small></span></button>)}
        </section>
        <footer><div className="notebook-user">IM</div><span><strong>Ivan</strong><small>Daemon online</small></span><Settings size={15} /></footer>
      </aside>

      <main className="notebook-page">
        <header className="notebook-page-header">
          <div className="notebook-crumb"><span>Dire Agent WebUI</span><ChevronRight size={13} /><span>Design lab</span></div>
          <div><button><ShieldCheck size={14} /> Ask before changes</button><button aria-label="More"><MoreHorizontal size={17} /></button></div>
        </header>
        <div className="notebook-document">
          <span className="notebook-date">JULY 12 · ACTIVE SESSION</span>
          <h2>Explore the WebUI design space</h2>
          <p className="notebook-lede">A working journal for the ten interface directions, their tradeoffs, and the evidence behind them.</p>
          <div className="notebook-meta"><span><Bot size={14} /> 4 agents</span><span><Clock3 size={14} /> 28 minutes</span><span><CircleDot size={14} /> Running</span></div>

          <section className="notebook-section">
            <header><span className="notebook-section-number">01</span><div><strong>Brief</strong><small>You · 10:14</small></div></header>
            <blockquote>{conversation[0].text}</blockquote>
          </section>

          <section className="notebook-section">
            <header><span className="notebook-agent-mark"><Sparkles size={14} /></span><div><strong>Orchestrator</strong><small>Working now</small></div></header>
            <p>{conversation[1].text}</p>
            <div className="notebook-plan">
              <header><span><ListChecks size={15} /> Implementation plan</span><strong>3 / 4</strong></header>
              {steps.map((step) => <div key={step.label} className={step.state}><i>{step.state === "done" && <Check size={11} />}</i><span>{step.label}</span>{step.state === "active" && <small>in progress</small>}</div>)}
            </div>
          </section>

          <details className="notebook-tool" open>
            <summary><span><SquareTerminal size={15} /> Build finished</span><span>3.8s <ChevronRight size={14} /></span></summary>
            <pre>✓ typecheck{`\n`}✓ 68 tests{`\n`}✓ production bundle</pre>
          </details>

          <section className="notebook-checkpoint">
            <RotateCcw size={16} /><span><strong>Checkpoint 8 saved</strong><small>Before responsive validation · 10:42</small></span><button>Restore</button>
          </section>
        </div>
        <div className="notebook-compose">
          <textarea aria-label="Continue the journal" rows={2} placeholder="Continue the work…" />
          <footer><span><button><Paperclip size={15} /> Add context</button><button>{view === "journal" ? "Build" : "Discuss"}<ChevronRight size={12} /></button></span><button className="notebook-send" aria-label="Send" onClick={() => setView(view === "journal" ? "discuss" : "journal")}><ArrowRight size={16} /></button></footer>
        </div>
      </main>

      <aside className="notebook-margin">
        <header><span>SESSION</span><button aria-label="Close panel">×</button></header>
        <section><h3>Contents</h3><button className="active"><span>01</span>Brief</button><button><span>02</span>Implementation</button><button><span>03</span>Validation</button></section>
        <section><h3>Artifacts</h3>{changes.map((change) => <button key={change.file}><FileCode2 size={14} /><span>{change.file.split("/").at(-1)}</span><small>{change.delta}</small></button>)}</section>
        <section><h3>Context</h3><div className="notebook-context-ring"><strong>11%</strong><span>42k / 372k</span></div><p>Plenty of room remains in this session.</p></section>
      </aside>
    </div>
  );
}

export function MobileCompanion() {
  const [section, setSection] = useState("home");
  const activeAgent = agents[0];

  return (
    <div className="mobile-concept">
      <div className="mobile-context-copy">
        <span><Smartphone size={15} /> MOBILE COMPANION</span>
        <h2>Monitor and steer.<br />Leave the workstation at your desk.</h2>
        <p>This direction deliberately removes terminals and configuration from mobile. It focuses on decisions, progress, and quick handoff.</p>
        <div><span><Check size={14} /> Approval-first</span><span><Check size={14} /> Thumb-friendly</span><span><Check size={14} /> Glanceable status</span></div>
      </div>
      <div className="mobile-device">
        <div className="mobile-island" />
        <header className="mobile-header">
          <div className="mobile-mini-avatar">IM</div>
          <span><strong>{section === "home" ? "Your workspace" : section === "runs" ? "Active runs" : "Inbox"}</strong><small>Dire Agent · online</small></span>
          <button aria-label="Notifications"><Bell size={18} /><i /></button>
        </header>

        <main className="mobile-content">
          {section === "home" && (
            <>
              <section className="mobile-hero">
                <span><Radio size={13} /> 2 AGENTS WORKING</span>
                <h3>Design lab is moving</h3>
                <p>{activeAgent.role}. The next update is expected in about six minutes.</p>
                <div><i><span style={{ width: `${activeAgent.progress}%` }} /></i><strong>{activeAgent.progress}%</strong></div>
                <button onClick={() => setSection("runs")}>Open live run <ArrowRight size={15} /></button>
              </section>
              <section className="mobile-section">
                <header><h3>Needs your attention</h3><span>1</span></header>
                <button className="mobile-decision" onClick={() => setSection("inbox")}>
                  <div><span className="mobile-warning"><Bell size={15} /></span><span><small>QA · 8 MIN AGO</small><strong>Choose the mockup URL structure</strong></span></div>
                  <p>The build can expose one switchable gallery or ten direct routes.</p>
                  <footer><span>Reply now</span><ChevronRight size={16} /></footer>
                </button>
              </section>
              <section className="mobile-section">
                <header><h3>Recent</h3><button>See all</button></header>
                {projects.slice(0, 2).map((project) => <button className="mobile-project" key={project.name}><span className={`mobile-project-icon ${project.status}`}><Folder size={16} /></span><span><strong>{project.name}</strong><small>{project.meta}</small></span><ChevronRight size={16} /></button>)}
              </section>
            </>
          )}
          {section === "runs" && (
            <>
              <section className="mobile-run-head"><button onClick={() => setSection("home")}>‹</button><div><span>Dire Agent WebUI</span><h3>Build ten interface directions</h3></div><MoreHorizontal size={18} /></section>
              <div className="mobile-run-agent"><span><Bot size={18} /></span><div><strong>orchestrator</strong><small><i /> Working now</small></div><strong>{activeAgent.progress}%</strong></div>
              <section className="mobile-run-plan"><header><span>Plan</span><strong>3 of 4</strong></header>{steps.map((step) => <div className={step.state} key={step.label}><i>{step.state === "done" && <Check size={10} />}</i><span>{step.label}</span></div>)}</section>
              <section className="mobile-live"><header><span><Activity size={14} /> Live activity</span><small>NOW</small></header>{activity.map((item) => <div key={item.time}><i /><span><strong>{item.text}</strong><small>{item.agent} · {item.time}</small></span></div>)}</section>
              <div className="mobile-steer"><input aria-label="Steer active run" placeholder="Steer this run…" /><button aria-label="Send steering message"><Send size={15} /></button></div>
            </>
          )}
          {section === "inbox" && (
            <>
              <section className="mobile-run-head"><button onClick={() => setSection("home")}>‹</button><div><span>Question from qa</span><h3>Mockup URL structure</h3></div><MoreHorizontal size={18} /></section>
              <article className="mobile-message"><span className="mobile-warning"><Bot size={15} /></span><div><small>QA · 8 MIN AGO</small><p>The build can expose one switchable gallery or ten direct routes. Which review flow should I optimize for?</p></div></article>
              <div className="mobile-options"><button>A switchable gallery <Check size={16} /></button><button>Ten direct routes <ChevronRight size={16} /></button></div>
              <div className="mobile-steer"><input aria-label="Reply to qa" placeholder="Add a note…" /><button aria-label="Send reply"><Send size={15} /></button></div>
            </>
          )}
        </main>

        <nav className="mobile-nav" aria-label="Mobile navigation">
          <button className={section === "home" ? "active" : ""} onClick={() => setSection("home")}><Home size={18} /><span>Home</span></button>
          <button className={section === "runs" ? "active" : ""} onClick={() => setSection("runs")}><Radio size={18} /><span>Runs</span><i>2</i></button>
          <button className={section === "inbox" ? "active" : ""} onClick={() => setSection("inbox")}><MessageCircle size={18} /><span>Inbox</span><i>1</i></button>
          <button><UserRound size={18} /><span>You</span></button>
        </nav>
      </div>
    </div>
  );
}

