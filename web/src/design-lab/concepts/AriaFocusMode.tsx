import {
  Activity,
  Bot,
  Check,
  ChevronDown,
  CircleStop,
  FileCode2,
  FolderKanban,
  Menu,
  PanelRight,
  Search,
  Send,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  Wrench,
  X,
} from "lucide-react";
import { useState } from "react";
import {
  Button,
  Dialog,
  DialogTrigger,
  Heading,
  Input,
  Label,
  ListBox,
  ListBoxItem,
  Modal,
  ModalOverlay,
  Popover,
  SearchField,
  Select,
  SelectValue,
  Switch,
  Tab,
  TabList,
  TabPanel,
  TabPanels,
  Tabs,
  TextArea,
} from "react-aria-components";
import { agents, changes, conversation, projects, steps } from "../mockData";
import "./aria-focus-mode.css";

const models = ["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"];

export function AriaFocusMode() {
  const [query, setQuery] = useState("");
  const [selectedProject, setSelectedProject] = useState<string>(projects[0].name);
  const [selectedAgent, setSelectedAgent] = useState<string>(agents[0].id);
  const [activeTab, setActiveTab] = useState("conversation");
  const [model, setModel] = useState(models[0]);
  const [composer, setComposer] = useState("");
  const [announceActivity, setAnnounceActivity] = useState(true);
  const [sentMessage, setSentMessage] = useState("");

  const currentAgent = agents.find((agent) => agent.id === selectedAgent) ?? agents[0];

  const sendMessage = () => {
    const value = composer.trim();
    if (!value) return;
    setSentMessage(value);
    setComposer("");
  };

  return (
    <div className="aria-focus" data-theme="focus-dark">
      <a className="aria-focus__skip" href="#aria-focus-content">Skip to conversation</a>

      <header className="aria-focus__header">
        <div className="aria-focus__header-start">
          <MobileNavigation
            query={query}
            selectedProject={selectedProject}
            selectedAgent={selectedAgent}
            onQueryChange={setQuery}
            onProjectChange={setSelectedProject}
            onAgentChange={setSelectedAgent}
          />
          <div className="aria-focus__brand" aria-label="Dire Agent">
            <span className="aria-focus__brand-mark"><TerminalSquare aria-hidden="true" size={18} /></span>
            <span><strong>Dire Agent</strong><small>Focus workspace</small></span>
          </div>
        </div>
        <div className="aria-focus__scope" aria-label="Current project scope">
          <span>Project</span>
          <strong>{selectedProject}</strong>
        </div>
        <div className="aria-focus__header-actions">
          <span className="aria-focus__connection"><i aria-hidden="true" /> Daemon online</span>
          <RunDetailsDialog
            announceActivity={announceActivity}
            currentAgent={currentAgent}
            onAnnounceActivityChange={setAnnounceActivity}
          />
        </div>
      </header>

      <div className="aria-focus__workspace">
        <aside className="aria-focus__sidebar" aria-label="Workspace navigation">
          <ProjectNavigator
            query={query}
            selectedProject={selectedProject}
            selectedAgent={selectedAgent}
            onQueryChange={setQuery}
            onProjectChange={setSelectedProject}
            onAgentChange={setSelectedAgent}
          />
        </aside>

        <main className="aria-focus__main" id="aria-focus-content">
          <Tabs
            className="aria-focus__tabs"
            selectedKey={activeTab}
            onSelectionChange={(key) => setActiveTab(String(key))}
          >
            <div className="aria-focus__tab-bar">
              <TabList aria-label="Project workspace views" className="aria-focus__tab-list">
                <Tab id="conversation" className="aria-focus__tab">Conversation</Tab>
                <Tab id="plan" className="aria-focus__tab">Plan <span>{steps.length}</span></Tab>
                <Tab id="changes" className="aria-focus__tab">Changes <span>{changes.length}</span></Tab>
              </TabList>
              <span className="aria-focus__shortcut" aria-hidden="true">⌘ K to navigate</span>
            </div>

            <TabPanels className="aria-focus__panels">
              <TabPanel id="conversation" className="aria-focus__panel">
                <section className="aria-focus__conversation" aria-labelledby="aria-focus-title">
                  <div className="aria-focus__conversation-heading">
                    <div>
                      <span className="aria-focus__eyebrow">PROJECT CONVERSATION</span>
                      <h1 id="aria-focus-title">Make parallel work easier to scan</h1>
                      <p>One quiet reading surface, with live work announced only when it changes.</p>
                    </div>
                    <span className="aria-focus__run-state"><Activity aria-hidden="true" size={15} /> Running · 3 agents</span>
                  </div>

                  <div className="aria-focus__messages" aria-live="polite">
                    {conversation.map((message, index) => (
                      <article className={`aria-focus__message aria-focus__message--${message.role}`} key={`${message.role}-${index}`}>
                        <div className="aria-focus__avatar" aria-hidden="true">
                          {message.role === "you" ? "Y" : <Bot size={16} />}
                        </div>
                        <div>
                          <header>
                            <strong>{message.role === "you" ? "You" : "orchestrator"}</strong>
                            <span>{index === 0 ? "10:31" : "10:34"}</span>
                          </header>
                          <p>{message.text}</p>
                        </div>
                      </article>
                    ))}

                    <article className="aria-focus__tool-update" aria-label="Tool activity: ui-builder is applying a patch, 54 percent complete">
                      <span className="aria-focus__tool-icon"><Wrench aria-hidden="true" size={15} /></span>
                      <div>
                        <header><strong>ui-builder is applying the responsive shell</strong><span>54%</span></header>
                        <p>Editing the comparison workspace · 2 files changed</p>
                        <span className="aria-focus__progress" aria-hidden="true"><i style={{ width: "54%" }} /></span>
                      </div>
                    </article>

                    {sentMessage && (
                      <article className="aria-focus__message aria-focus__message--you aria-focus__message--sent">
                        <div className="aria-focus__avatar" aria-hidden="true">Y</div>
                        <div><header><strong>You</strong><span>just now</span></header><p>{sentMessage}</p></div>
                      </article>
                    )}
                  </div>

                  <form
                    className="aria-focus__composer"
                    onSubmit={(event) => { event.preventDefault(); sendMessage(); }}
                  >
                    <Label className="aria-focus__sr-only" htmlFor="aria-focus-message">Message the agent</Label>
                    <TextArea
                      id="aria-focus-message"
                      value={composer}
                      onChange={(event) => setComposer(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" && !event.shiftKey) {
                          event.preventDefault();
                          sendMessage();
                        }
                      }}
                      placeholder="Message the agent, or type / for commands…"
                      rows={3}
                    />
                    <div className="aria-focus__composer-actions">
                      <Select
                        aria-label="Model"
                        className="aria-focus__select"
                        selectedKey={model}
                        onSelectionChange={(key) => setModel(String(key))}
                      >
                        <Button className="aria-focus__select-trigger">
                          <Sparkles aria-hidden="true" size={13} />
                          <SelectValue />
                          <ChevronDown aria-hidden="true" size={13} />
                        </Button>
                        <Popover className="aria-focus__select-popover">
                          <ListBox className="aria-focus__select-list" items={models.map((name) => ({ id: name, name }))}>
                            {(item) => <ListBoxItem className="aria-focus__select-option" id={item.id}>{item.name}</ListBoxItem>}
                          </ListBox>
                        </Popover>
                      </Select>
                      <span className="aria-focus__composer-hint">Enter to send · Shift + Enter for a new line</span>
                      <Button className="aria-focus__send" type="submit" isDisabled={!composer.trim()}>
                        <Send aria-hidden="true" size={15} /> Send
                      </Button>
                    </div>
                  </form>
                </section>
              </TabPanel>

              <TabPanel id="plan" className="aria-focus__panel">
                <section className="aria-focus__document" aria-labelledby="aria-plan-title">
                  <span className="aria-focus__eyebrow">LIVE PLAN</span>
                  <h1 id="aria-plan-title">A short, legible path to review</h1>
                  <p>Progress is expressed with text and shape, so state never depends on color alone.</p>
                  <ol className="aria-focus__plan">
                    {steps.map((step, index) => (
                      <li className={`aria-focus__plan-item aria-focus__plan-item--${step.state}`} key={step.label}>
                        <span aria-hidden="true">{step.state === "done" ? <Check size={15} /> : index + 1}</span>
                        <div><strong>{step.label}</strong><small>{step.state === "active" ? "In progress" : step.state}</small></div>
                      </li>
                    ))}
                  </ol>
                </section>
              </TabPanel>

              <TabPanel id="changes" className="aria-focus__panel">
                <section className="aria-focus__document" aria-labelledby="aria-changes-title">
                  <span className="aria-focus__eyebrow">REVIEW SURFACE</span>
                  <h1 id="aria-changes-title">Three focused changes</h1>
                  <p>Files stay secondary to the task until review is requested.</p>
                  <div className="aria-focus__change-list">
                    {changes.map((change) => (
                      <button type="button" key={change.file}>
                        <FileCode2 aria-hidden="true" size={17} />
                        <span><strong>{change.file}</strong><small>{change.state}</small></span>
                        <em>{change.delta}</em>
                      </button>
                    ))}
                  </div>
                </section>
              </TabPanel>
            </TabPanels>
          </Tabs>
        </main>
      </div>
    </div>
  );
}

function ProjectNavigator(props: {
  query: string;
  selectedProject: string;
  selectedAgent: string;
  onQueryChange: (value: string) => void;
  onProjectChange: (value: string) => void;
  onAgentChange: (value: string) => void;
}) {
  const normalized = props.query.trim().toLowerCase();
  const visibleProjects = normalized
    ? projects.filter((project) => `${project.name} ${project.meta}`.toLowerCase().includes(normalized))
    : projects;

  return (
    <div className="aria-focus__navigator">
      <SearchField className="aria-focus__search" value={props.query} onChange={props.onQueryChange} aria-label="Search projects">
        <Search aria-hidden="true" size={15} />
        <Input placeholder="Search projects" />
        {props.query && <Button aria-label="Clear search"><X aria-hidden="true" size={13} /></Button>}
      </SearchField>

      <div className="aria-focus__nav-heading"><span><FolderKanban aria-hidden="true" size={14} /> Projects</span><kbd>4</kbd></div>
      <ListBox
        aria-label="Projects"
        className="aria-focus__project-list"
        selectionMode="single"
        selectedKeys={[props.selectedProject]}
        onSelectionChange={(keys) => {
          if (keys === "all") return;
          const key = [...keys][0];
          if (key !== undefined) props.onProjectChange(String(key));
        }}
      >
        {visibleProjects.map((project) => (
          <ListBoxItem className="aria-focus__project" id={project.name} textValue={project.name} key={project.name}>
            <span className={`aria-focus__project-state aria-focus__project-state--${project.status}`} aria-hidden="true" />
            <span><strong>{project.name}</strong><small>{project.meta}</small></span>
          </ListBoxItem>
        ))}
      </ListBox>

      <div className="aria-focus__nav-heading aria-focus__nav-heading--agents"><span><Bot aria-hidden="true" size={14} /> Agents</span><kbd>3 live</kbd></div>
      <ListBox
        aria-label="Agents"
        className="aria-focus__agent-list"
        selectionMode="single"
        selectedKeys={[props.selectedAgent]}
        onSelectionChange={(keys) => {
          if (keys === "all") return;
          const key = [...keys][0];
          if (key !== undefined) props.onAgentChange(String(key));
        }}
      >
        {agents.map((agent) => (
          <ListBoxItem className="aria-focus__agent" id={agent.id} textValue={agent.name} key={agent.id}>
            <span className={`aria-focus__agent-dot aria-focus__agent-dot--${agent.status.toLowerCase()}`} aria-hidden="true" />
            <span><strong>{agent.name}</strong><small>{agent.status} · {agent.progress}%</small></span>
          </ListBoxItem>
        ))}
      </ListBox>

      <div className="aria-focus__trust"><ShieldCheck aria-hidden="true" size={15} /><span><strong>Workspace sandbox</strong><small>Read, grep, find, ls</small></span></div>
    </div>
  );
}

function MobileNavigation(props: Parameters<typeof ProjectNavigator>[0]) {
  return (
    <DialogTrigger>
      <Button className="aria-focus__icon-button aria-focus__mobile-menu" aria-label="Open workspace navigation"><Menu size={18} /></Button>
      <ModalOverlay className="aria-focus__overlay" isDismissable>
        <Modal className="aria-focus__modal aria-focus__modal--navigation">
          <Dialog className="aria-focus__dialog">
            {({ close }) => (
              <>
                <header><Heading slot="title">Workspace</Heading><Button className="aria-focus__icon-button" onPress={close} aria-label="Close navigation"><X size={17} /></Button></header>
                <ProjectNavigator {...props} />
              </>
            )}
          </Dialog>
        </Modal>
      </ModalOverlay>
    </DialogTrigger>
  );
}

function RunDetailsDialog(props: {
  announceActivity: boolean;
  currentAgent: (typeof agents)[number];
  onAnnounceActivityChange: (value: boolean) => void;
}) {
  return (
    <DialogTrigger>
      <Button className="aria-focus__details-button"><PanelRight aria-hidden="true" size={16} /><span>Run details</span></Button>
      <ModalOverlay className="aria-focus__overlay" isDismissable>
        <Modal className="aria-focus__modal aria-focus__modal--details">
          <Dialog className="aria-focus__dialog aria-focus__details-dialog">
            {({ close }) => (
              <>
                <header><div><span className="aria-focus__eyebrow">CURRENT RUN</span><Heading slot="title">Control without clutter</Heading></div><Button className="aria-focus__icon-button" onPress={close} aria-label="Close run details"><X size={17} /></Button></header>
                <div className="aria-focus__details-scroll">
                  <section className="aria-focus__detail-agent">
                    <span className="aria-focus__detail-avatar"><Bot size={18} /></span>
                    <div><strong>{props.currentAgent.name}</strong><small>{props.currentAgent.role}</small></div>
                    <span>{props.currentAgent.progress}%</span>
                  </section>
                  <section className="aria-focus__context-meter" aria-label="Context window 41 percent used">
                    <header><span>Context window</span><strong>153k / 372k</strong></header>
                    <span aria-hidden="true"><i /></span>
                  </section>
                  <Switch className="aria-focus__switch" isSelected={props.announceActivity} onChange={props.onAnnounceActivityChange}>
                    <span className="aria-focus__switch-track"><i /></span>
                    <span><strong>Announce live activity</strong><small>Screen readers hear meaningful state changes.</small></span>
                  </Switch>
                  <section className="aria-focus__detail-section">
                    <h3>Queue</h3>
                    <div><Activity size={15} /><span><strong>Follow-up</strong><small>1 message waiting</small></span><em>next</em></div>
                    <div><CircleStop size={15} /><span><strong>Steering</strong><small>No interruption queued</small></span><em>clear</em></div>
                  </section>
                  <section className="aria-focus__detail-section">
                    <h3>Trust boundary</h3>
                    <div><ShieldCheck size={15} /><span><strong>Workspace mode</strong><small>Network denied · 1 writable root</small></span><em>safe</em></div>
                  </section>
                </div>
              </>
            )}
          </Dialog>
        </Modal>
      </ModalOverlay>
    </DialogTrigger>
  );
}

export default AriaFocusMode;
