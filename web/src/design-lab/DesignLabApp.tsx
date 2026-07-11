import {
  ArrowLeft,
  ArrowRight,
  Check,
  ChevronDown,
  Command,
  Columns2,
  ExternalLink,
  Grid2X2,
  Laptop,
  ListFilter,
  Monitor,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
  Smartphone,
  Sparkles,
  Tablet,
} from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import type { DesignConcept } from "./types";
import "./design-lab.css";

const ShadcnCommandCenter = lazy(() => import("./concepts/ShadcnCommandCenter").then((module) => ({ default: module.ShadcnCommandCenter })));
const MUIOperationsCockpit = lazy(() => import("./concepts/MUIOperationsCockpit").then((module) => ({ default: module.MUIOperationsCockpit })));
const MantineWorkbench = lazy(() => import("./concepts/MantineWorkbench").then((module) => ({ default: module.MantineWorkbench })));
const AntFleetConsole = lazy(() => import("./concepts/AntFleetConsole").then((module) => ({ default: module.AntFleetConsole })));
const AriaFocusMode = lazy(() => import("./concepts/AriaFocusMode").then((module) => ({ default: module.AriaFocusMode })));
const FlowTopology = lazy(() => import("./concepts/FlowTopology").then((module) => ({ default: module.FlowTopology })));
const AttentionInbox = lazy(() => import("./concepts/CustomConcepts").then((module) => ({ default: module.AttentionInbox })));
const TerminalIDE = lazy(() => import("./concepts/CustomConcepts").then((module) => ({ default: module.TerminalIDE })));
const NotebookWorkspace = lazy(() => import("./concepts/CustomConcepts").then((module) => ({ default: module.NotebookWorkspace })));
const MobileCompanion = lazy(() => import("./concepts/CustomConcepts").then((module) => ({ default: module.MobileCompanion })));

const concepts: DesignConcept[] = [
  {
    id: "command-center",
    number: "01",
    name: "Command Center",
    shortName: "Command",
    library: "shadcn/ui",
    thesis: "A balanced production candidate with chat at the center, live work always visible, and advanced controls one layer away.",
    bestFor: "Everyday power users",
    accent: "#7dd3fc",
    Component: ShadcnCommandCenter,
  },
  {
    id: "operations-cockpit",
    number: "02",
    name: "Operations Cockpit",
    shortName: "Cockpit",
    library: "Material UI",
    thesis: "Treat long-running agents as an operational system with strong progress, health, alerts, and recoverable checkpoints.",
    bestFor: "Monitoring many live runs",
    accent: "#5b8cff",
    Component: MUIOperationsCockpit,
  },
  {
    id: "three-pane-workbench",
    number: "03",
    name: "Three-Pane Workbench",
    shortName: "Workbench",
    library: "Mantine",
    thesis: "A familiar developer workbench that separates project navigation, the active artifact, and contextual inspection.",
    bestFor: "Deep project sessions",
    accent: "#45c4a0",
    Component: MantineWorkbench,
  },
  {
    id: "fleet-console",
    number: "04",
    name: "Fleet Console",
    shortName: "Fleet",
    library: "Ant Design",
    thesis: "A global control plane organized around lifecycle, health, ownership, and review across a large agent fleet.",
    bestFor: "Teams and many projects",
    accent: "#8b7cf6",
    Component: AntFleetConsole,
  },
  {
    id: "focus-mode",
    number: "05",
    name: "Accessible Focus Mode",
    shortName: "Focus",
    library: "React Aria",
    thesis: "A high-contrast, keyboard-first interface with one dominant task, generous type, and no hidden interaction targets.",
    bestFor: "Focused and accessible work",
    accent: "#f6c85f",
    Component: AriaFocusMode,
  },
  {
    id: "agent-topology",
    number: "06",
    name: "Agent Topology",
    shortName: "Topology",
    library: "React Flow",
    thesis: "Make delegation legible as a live map while keeping the graph observational and the conversation close at hand.",
    bestFor: "Complex multi-agent work",
    accent: "#f97389",
    Component: FlowTopology,
  },
  {
    id: "attention-inbox",
    number: "07",
    name: "Attention Inbox",
    shortName: "Inbox",
    library: "Custom React",
    thesis: "Organize asynchronous work by what is running, blocked, or ready for review—not by which chat was opened last.",
    bestFor: "Async delegation and triage",
    accent: "#5f7fec",
    Component: AttentionInbox,
  },
  {
    id: "terminal-ide",
    number: "08",
    name: "Terminal-First IDE",
    shortName: "Terminal",
    library: "React + xterm language",
    thesis: "Lean into Dire Agent’s technical identity with durable editor groups, a first-class terminal, and a docked agent console.",
    bestFor: "Keyboard-heavy coding",
    accent: "#ff795e",
    Component: TerminalIDE,
  },
  {
    id: "notebook-workspace",
    number: "09",
    name: "Notebook Workspace",
    shortName: "Notebook",
    library: "Custom React",
    thesis: "Turn a run into a readable working record where plans, tool calls, checkpoints, and artifacts form one coherent document.",
    bestFor: "Long-form thinking and audit",
    accent: "#71806b",
    Component: NotebookWorkspace,
  },
  {
    id: "mobile-companion",
    number: "10",
    name: "Mobile Companion",
    shortName: "Mobile",
    library: "Custom React",
    thesis: "A purpose-built monitor and steering surface for decisions on the move, without squeezing a desktop IDE into a phone.",
    bestFor: "Monitoring away from desk",
    accent: "#819cff",
    Component: MobileCompanion,
  },
];

type Viewport = "desktop" | "tablet" | "mobile";

function pathConcept(): string {
  const match = window.location.pathname.match(/^\/designs\/([^/]+)/);
  return match?.[1] && concepts.some((concept) => concept.id === match[1]) ? match[1] : "";
}

export function DesignLabApp() {
  const [selectedID, setSelectedID] = useState(pathConcept);
  const [viewport, setViewport] = useState<Viewport>("desktop");
  const [railOpen, setRailOpen] = useState(true);
  const [query, setQuery] = useState("");
  const selected = concepts.find((concept) => concept.id === selectedID) ?? null;
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return concepts;
    return concepts.filter((concept) => `${concept.name} ${concept.library} ${concept.thesis} ${concept.bestFor}`.toLowerCase().includes(needle));
  }, [query]);

  const navigate = useCallback((id: string, replace = false) => {
    setSelectedID(id);
    const url = id ? `/designs/${id}` : "/designs";
    window.history[replace ? "replaceState" : "pushState"]({}, "", url);
  }, []);

  useEffect(() => {
    const handlePopState = () => setSelectedID(pathConcept());
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  useEffect(() => {
    document.title = selected ? `${selected.number} ${selected.name} · Dire Agent UI Lab` : "Dire Agent UI Lab · 10 WebUI directions";
  }, [selected]);

  const selectedIndex = selected ? concepts.findIndex((concept) => concept.id === selected.id) : -1;
  const move = (offset: number) => {
    const nextIndex = (selectedIndex + offset + concepts.length) % concepts.length;
    navigate(concepts[nextIndex].id);
  };

  return (
    <div className={`design-lab ${railOpen ? "rail-open" : "rail-closed"} ${selected ? "has-selection" : "is-overview"}`}>
      <header className="design-lab-header">
        <button className="design-lab-rail-toggle" onClick={() => setRailOpen((value) => !value)} aria-label={railOpen ? "Collapse concept navigation" : "Open concept navigation"}>
          {railOpen ? <PanelLeftClose size={17} /> : <PanelLeftOpen size={17} />}
        </button>
        <button className="design-lab-brand" onClick={() => navigate("")}>
          <span><Command size={17} /></span>
          <strong>Dire Agent <i>UI Lab</i></strong>
        </button>
        <div className="design-lab-header-note"><Sparkles size={14} /><span>10 product directions · same task, same data</span></div>
        <a href="/" className="design-lab-exit">Live app <ExternalLink size={13} /></a>
      </header>

      <aside className="design-lab-rail" aria-label="Design concepts">
        <div className="design-lab-rail-head">
          <div><span>DESIGN SPACE</span><strong>Ten directions</strong></div>
          <button onClick={() => navigate("")} className={!selected ? "active" : ""} aria-label="Show all concepts"><Grid2X2 size={16} /></button>
        </div>
        <label className="design-lab-search">
          <Search size={14} />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter concepts" aria-label="Filter design concepts" />
        </label>
        <nav>
          {filtered.map((concept) => (
            <button
              key={concept.id}
              className={selected?.id === concept.id ? "active" : ""}
              onClick={() => navigate(concept.id)}
              style={{ "--concept-accent": concept.accent } as React.CSSProperties}
            >
              <span className="design-lab-number">{concept.number}</span>
              <span><strong>{concept.name}</strong><small>{concept.library}</small></span>
              {selected?.id === concept.id && <Check size={13} />}
            </button>
          ))}
        </nav>
        <div className="design-lab-rail-foot">
          <ListFilter size={14} />
          <span><strong>{filtered.length} of 10</strong><small>Library-backed and custom</small></span>
        </div>
      </aside>

      <main className="design-lab-main">
        {!selected ? (
          <ConceptOverview onSelect={(id) => navigate(id)} />
        ) : (
          <>
            <header className="design-concept-header">
              <div className="design-concept-title">
                <button className="design-concept-back" onClick={() => navigate("")} aria-label="Back to all concepts"><ArrowLeft size={16} /></button>
                <span className="design-concept-index" style={{ color: selected.accent }}>{selected.number}</span>
                <div><h1>{selected.name}</h1><span>{selected.library} · {selected.bestFor}</span></div>
              </div>
              <div className="design-viewport-controls" aria-label="Preview viewport">
                {([
                  ["desktop", <Monitor size={14} />, "Desktop"],
                  ["tablet", <Tablet size={14} />, "Tablet"],
                  ["mobile", <Smartphone size={14} />, "Mobile"],
                ] as const).map(([value, icon, label]) => (
                  <button key={value} className={viewport === value ? "active" : ""} onClick={() => setViewport(value)} aria-pressed={viewport === value} aria-label={`${label} viewport`}>{icon}<span>{label}</span></button>
                ))}
              </div>
              <div className="design-concept-pager">
                <button onClick={() => move(-1)} aria-label="Previous concept"><ArrowLeft size={15} /></button>
                <span>{selectedIndex + 1} / {concepts.length}</span>
                <button onClick={() => move(1)} aria-label="Next concept"><ArrowRight size={15} /></button>
              </div>
            </header>

            <div className="design-concept-thesis"><span style={{ background: selected.accent }} /><p>{selected.thesis}</p></div>

            <section className={`design-preview-wrap viewport-${viewport}`} aria-label={`${selected.name} interactive mockup`}>
              <div className="design-preview-device">
                <div className="design-preview-chrome">
                  <span><i /><i /><i /></span>
                  <div><Laptop size={12} /> dire.local/{selected.shortName.toLowerCase()}</div>
                  <span />
                </div>
                <div className="design-preview-surface">
                  <Suspense fallback={<div className="design-concept-loading"><Sparkles size={17} /> Preparing concept…</div>}>
                    <selected.Component />
                  </Suspense>
                </div>
              </div>
            </section>
          </>
        )}
      </main>

      <div className="design-mobile-picker">
        <label>
          <span>{selected ? `${selected.number} · ${selected.name}` : "All concepts"}</span>
          <select value={selected?.id ?? ""} onChange={(event) => navigate(event.target.value)} aria-label="Choose a design concept">
            <option value="">All ten concepts</option>
            {concepts.map((concept) => <option value={concept.id} key={concept.id}>{concept.number} · {concept.name} · {concept.library}</option>)}
          </select>
          <ChevronDown size={14} />
        </label>
      </div>
    </div>
  );
}

function ConceptOverview({ onSelect }: { onSelect: (id: string) => void }) {
  return (
    <div className="design-overview">
      <header>
        <span className="design-overview-kicker"><Columns2 size={14} /> PRODUCT EXPLORATION</span>
        <h1>Ten ways to make parallel agent work feel legible.</h1>
        <p>Each direction uses the same project, agents, plan, changes, and review state. Open any concept, interact with it, then test its desktop, tablet, and mobile behavior.</p>
        <div className="design-overview-legend"><span><i className="library" /> 6 library-backed</span><span><i className="custom" /> 4 product-specific</span><span><i className="research" /> 9 comparable products reviewed</span></div>
      </header>
      <div className="design-overview-grid">
        {concepts.map((concept, index) => (
          <button key={concept.id} onClick={() => onSelect(concept.id)} style={{ "--concept-accent": concept.accent } as React.CSSProperties}>
            <div className={`design-mini design-mini-${(index % 5) + 1}`}>
              <i /><i /><i /><i /><i /><i />
            </div>
            <div className="design-overview-card-head"><span>{concept.number}</span><small>{concept.library}</small></div>
            <h2>{concept.name}</h2>
            <p>{concept.thesis}</p>
            <footer><span>{concept.bestFor}</span><ArrowRight size={15} /></footer>
          </button>
        ))}
      </div>
    </div>
  );
}

export default DesignLabApp;
