import "@mantine/core/styles.css";
import {
  ActionIcon,
  Avatar,
  Badge,
  Button,
  Card,
  Divider,
  Group,
  MantineProvider,
  Progress,
  ScrollArea,
  SegmentedControl,
  Stack,
  Tabs,
  Text,
  Textarea,
  ThemeIcon,
  Tooltip,
  createTheme,
} from "@mantine/core";
import {
  Activity,
  ArrowUp,
  Bot,
  Check,
  CheckCircle2,
  ChevronDown,
  CircleDot,
  Code2,
  Eye,
  FileCode2,
  GitBranch,
  Layers3,
  Menu,
  MessageSquareText,
  MoreHorizontal,
  PanelRight,
  Play,
  Plus,
  Search,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  X,
} from "lucide-react";
import { useMemo, useState } from "react";
import { agents, changes, conversation, projects, steps } from "../mockData";
import "./MantineWorkbench.css";

const workbenchTheme = createTheme({
  primaryColor: "violet",
  defaultRadius: "md",
  fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, sans-serif",
  headings: { fontFamily: "Inter, ui-sans-serif, system-ui, sans-serif" },
  colors: {
    violet: [
      "#f4f1ff",
      "#ebe5ff",
      "#d9ceff",
      "#bfa8ff",
      "#a67ff5",
      "#8b5ce8",
      "#7746dc",
      "#6234c4",
      "#512ca1",
      "#432683",
    ],
  },
});

const taskDetails = [
  { branch: "codex/design-lab", files: 3, context: 41, label: "Active build" },
  { branch: "fix/checkout-retry", files: 7, context: 73, label: "Review ready" },
  { branch: "docs/navigation", files: 2, context: 26, label: "Paused" },
  { branch: "chore/telemetry", files: 5, context: 18, label: "Completed" },
] as const;

const statusTone = {
  running: { color: "violet", label: "Running" },
  review: { color: "yellow", label: "Needs review" },
  idle: { color: "gray", label: "Idle" },
  done: { color: "teal", label: "Done" },
} as const;

function MantineWorkbenchInner() {
  const [selectedTask, setSelectedTask] = useState(0);
  const [surface, setSurface] = useState<string | null>("conversation");
  const [permission, setPermission] = useState("ask");
  const [composer, setComposer] = useState("");
  const [sentMessage, setSentMessage] = useState("");
  const [navOpen, setNavOpen] = useState(false);
  const [inspectorOpen, setInspectorOpen] = useState(true);
  const [changesApproved, setChangesApproved] = useState(false);
  const [previewSize, setPreviewSize] = useState("desktop");
  const selected = projects[selectedTask];
  const detail = taskDetails[selectedTask];
  const tone = statusTone[selected.status];

  const groupedTasks = useMemo(() => ({
    active: projects.map((project, index) => ({ project, index })).filter(({ project }) => project.status !== "done"),
    completed: projects.map((project, index) => ({ project, index })).filter(({ project }) => project.status === "done"),
  }), []);

  const sendMessage = () => {
    const value = composer.trim();
    if (!value) return;
    setSentMessage(value);
    setComposer("");
  };

  const selectTask = (index: number) => {
    setSelectedTask(index);
    setNavOpen(false);
    setChangesApproved(false);
  };

  return (
    <div className="mw-root">
      {navOpen && (
        <button className="mw-scrim" onClick={() => setNavOpen(false)} aria-label="Close task navigation" />
      )}

      <aside className={`mw-sidebar ${navOpen ? "is-open" : ""}`} aria-label="Project and task navigation">
        <div className="mw-brand">
          <ThemeIcon size={36} radius={12} color="violet" variant="filled"><Sparkles size={17} /></ThemeIcon>
          <div>
            <Text fw={750} size="sm">Dire Workbench</Text>
            <Text c="dimmed" size="xs">Local agent workspace</Text>
          </div>
          <ActionIcon className="mw-nav-close" variant="subtle" color="gray" onClick={() => setNavOpen(false)} aria-label="Close navigation">
            <X size={16} />
          </ActionIcon>
        </div>

        <button className="mw-project-switcher" type="button">
          <span className="mw-project-icon"><Layers3 size={16} /></span>
          <span>
            <strong>Dire Agent</strong>
            <small>ivan / dire-agent</small>
          </span>
          <ChevronDown size={14} />
        </button>

        <label className="mw-search">
          <Search size={14} />
          <input aria-label="Search project tasks" placeholder="Find a task…" />
          <kbd>⌘K</kbd>
        </label>

        <ScrollArea className="mw-task-scroll" type="auto">
          <nav className="mw-task-nav" aria-label="Tasks">
            <TaskGroup
              label="In progress"
              items={groupedTasks.active}
              selectedTask={selectedTask}
              onSelect={selectTask}
            />
            <TaskGroup
              label="Completed"
              items={groupedTasks.completed}
              selectedTask={selectedTask}
              onSelect={selectTask}
            />
          </nav>
        </ScrollArea>

        <div className="mw-sidebar-footer">
          <Button fullWidth variant="light" color="violet" leftSection={<Plus size={15} />}>New task</Button>
          <Group justify="space-between" wrap="nowrap">
            <Group gap={8} wrap="nowrap">
              <span className="mw-online-dot" />
              <div>
                <Text size="xs" fw={650}>Daemon online</Text>
                <Text size="xs" c="dimmed">Local · sandboxed</Text>
              </div>
            </Group>
            <ActionIcon variant="subtle" color="gray" aria-label="More connection options"><MoreHorizontal size={16} /></ActionIcon>
          </Group>
        </div>
      </aside>

      <section className="mw-shell">
        <header className="mw-topbar">
          <Group gap={10} wrap="nowrap" className="mw-heading-group">
            <ActionIcon className="mw-menu-button" variant="default" onClick={() => setNavOpen(true)} aria-label="Open task navigation">
              <Menu size={17} />
            </ActionIcon>
            <div className="mw-breadcrumb">
              <Text c="dimmed" size="xs">Dire Agent</Text>
              <span>/</span>
              <Text size="xs" fw={650}>{selected.name}</Text>
            </div>
          </Group>
          <Group gap={8} wrap="nowrap">
            <Badge variant="light" color={tone.color} leftSection={selected.status === "running" ? <Activity size={11} /> : <CircleDot size={11} />}>
              {tone.label}
            </Badge>
            <Tooltip label="Open task inspector">
              <ActionIcon
                variant={inspectorOpen ? "light" : "default"}
                color="violet"
                onClick={() => setInspectorOpen((value) => !value)}
                aria-label={inspectorOpen ? "Hide task inspector" : "Open task inspector"}
              >
                <PanelRight size={16} />
              </ActionIcon>
            </Tooltip>
          </Group>
        </header>

        <div className="mw-task-header">
          <div className="mw-title-block">
            <Group gap={8} wrap="wrap">
              <Text className="mw-eyebrow">{detail.label}</Text>
              <span className="mw-header-dot" />
              <Text c="dimmed" size="xs">updated 2m ago</Text>
            </Group>
            <Text component="h1" className="mw-task-title">{selected.name}</Text>
            <Group gap={14} className="mw-task-meta">
              <span><GitBranch size={13} />{detail.branch}</span>
              <span><FileCode2 size={13} />{detail.files} files changed</span>
              <span><ShieldCheck size={13} />Workspace sandbox</span>
            </Group>
          </div>
          <Group gap={8} wrap="nowrap" className="mw-header-actions">
            <Button variant="default" leftSection={<TerminalSquare size={15} />}><span className="mw-wide-label">Terminal</span></Button>
            <Button color="violet" leftSection={<Play size={15} />}>{selected.status === "running" ? "Steer run" : "Resume"}</Button>
          </Group>
        </div>

        <Tabs className="mw-tabs" value={surface} onChange={setSurface} keepMounted={false}>
          <Tabs.List className="mw-tab-list">
            <Tabs.Tab value="conversation" leftSection={<MessageSquareText size={15} />}>Conversation</Tabs.Tab>
            <Tabs.Tab value="changes" leftSection={<Code2 size={15} />} rightSection={<span className="mw-tab-count">3</span>}>Changes</Tabs.Tab>
            <Tabs.Tab value="preview" leftSection={<Eye size={15} />}>Preview</Tabs.Tab>
          </Tabs.List>

          <Tabs.Panel value="conversation" className="mw-tab-panel">
            <div className={`mw-conversation-grid ${inspectorOpen ? "has-inspector" : ""}`}>
              <section className="mw-chat" aria-label="Task conversation">
                <ScrollArea className="mw-chat-scroll" type="auto" offsetScrollbars>
                  <div className="mw-chat-inner">
                    <div className="mw-day-label"><span />Today, 10:31<span /></div>
                    {conversation.map((message) => (
                      <div className={`mw-message ${message.role}`} key={`${message.role}-${message.text}`}>
                        <Avatar size={30} radius="xl" color={message.role === "agent" ? "violet" : "dark"}>
                          {message.role === "agent" ? <Bot size={15} /> : "IM"}
                        </Avatar>
                        <div>
                          <Group gap={7} mb={5}>
                            <Text size="xs" fw={700}>{message.role === "agent" ? "Dire Agent" : "You"}</Text>
                            <Text size="xs" c="dimmed">10:{message.role === "agent" ? "33" : "31"}</Text>
                          </Group>
                          <Text size="sm" lh={1.58}>{message.text}</Text>
                        </div>
                      </div>
                    ))}

                    <div className="mw-tool-event">
                      <div className="mw-tool-head">
                        <ThemeIcon size={28} radius="xl" color="teal" variant="light"><Check size={14} /></ThemeIcon>
                        <div>
                          <Text size="xs" fw={700}>Workspace explored</Text>
                          <Text size="xs" c="dimmed">18 files read · 6 searches · 42s</Text>
                        </div>
                        <Badge color="teal" variant="light" size="sm">Complete</Badge>
                      </div>
                      <div className="mw-tool-code"><code>rg --files web/src/design-lab</code><span>exit 0</span></div>
                    </div>

                    <div className="mw-message agent">
                      <Avatar size={30} radius="xl" color="violet"><Bot size={15} /></Avatar>
                      <div>
                        <Group gap={7} mb={5}>
                          <Text size="xs" fw={700}>Dire Agent</Text>
                          <Text size="xs" c="dimmed">10:42</Text>
                        </Group>
                        <Text size="sm" lh={1.58}>The first concept is ready to review. I separated the task hierarchy from the live run, then gave Changes and Preview equal weight with the conversation.</Text>
                        <Group gap={8} mt={11}>
                          <Button size="xs" variant="light" color="violet" onClick={() => setSurface("changes")}>Review 3 files</Button>
                          <Button size="xs" variant="default" onClick={() => setSurface("preview")}>Open preview</Button>
                        </Group>
                      </div>
                    </div>

                    {sentMessage && (
                      <div className="mw-message you">
                        <Avatar size={30} radius="xl" color="dark">IM</Avatar>
                        <div>
                          <Group gap={7} mb={5}><Text size="xs" fw={700}>You</Text><Text size="xs" c="dimmed">now</Text></Group>
                          <Text size="sm" lh={1.58}>{sentMessage}</Text>
                        </div>
                      </div>
                    )}
                  </div>
                </ScrollArea>

                <div className="mw-composer-wrap">
                  <div className="mw-composer">
                    <Textarea
                      autosize
                      minRows={2}
                      maxRows={5}
                      variant="unstyled"
                      value={composer}
                      onChange={(event) => setComposer(event.currentTarget.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" && !event.shiftKey) {
                          event.preventDefault();
                          sendMessage();
                        }
                      }}
                      placeholder={selected.status === "running" ? "Steer the current run…" : "Message the agent…"}
                      aria-label="Message the agent"
                    />
                    <Divider />
                    <Group justify="space-between" gap={8} wrap="nowrap" className="mw-composer-bar">
                      <SegmentedControl
                        size="xs"
                        value={permission}
                        onChange={setPermission}
                        aria-label="Agent permission mode"
                        data={[
                          { label: "Discuss", value: "discuss" },
                          { label: "Ask first", value: "ask" },
                          { label: "Sandbox", value: "sandbox" },
                        ]}
                      />
                      <Group gap={6} wrap="nowrap">
                        <Tooltip label="Attach context"><ActionIcon variant="subtle" color="gray" aria-label="Attach context"><Plus size={16} /></ActionIcon></Tooltip>
                        <ActionIcon color="violet" variant="filled" onClick={sendMessage} disabled={!composer.trim()} aria-label="Send message"><ArrowUp size={16} /></ActionIcon>
                      </Group>
                    </Group>
                  </div>
                  <Text ta="center" size="xs" c="dimmed" mt={7}>Enter to send · Shift + Enter for a new line</Text>
                </div>
              </section>

              {inspectorOpen && (
                <aside className="mw-inspector" aria-label="Task plan and agents">
                  <ScrollArea className="mw-inspector-scroll" type="auto">
                    <Stack gap={18} p="md">
                      <section>
                        <Group justify="space-between" mb={12}>
                          <Text size="xs" fw={800} tt="uppercase" className="mw-section-label">Plan</Text>
                          <Text size="xs" c="dimmed">2 of 4</Text>
                        </Group>
                        <Stack gap={10}>
                          {steps.map((step, index) => (
                            <div className={`mw-step ${step.state}`} key={step.label}>
                              <span className="mw-step-icon">{step.state === "done" ? <Check size={12} /> : index + 1}</span>
                              <div><Text size="xs" fw={step.state === "active" ? 700 : 500}>{step.label}</Text>{step.state === "active" && <Progress value={64} size={3} mt={7} color="violet" />}</div>
                            </div>
                          ))}
                        </Stack>
                      </section>

                      <Divider />

                      <section>
                        <Group justify="space-between" mb={12}>
                          <Text size="xs" fw={800} tt="uppercase" className="mw-section-label">Agent team</Text>
                          <Avatar.Group spacing="sm">
                            {agents.slice(0, 3).map((agent) => <Avatar key={agent.id} size={24} color={agent.status === "Done" ? "teal" : "violet"}>{agent.name[0].toUpperCase()}</Avatar>)}
                          </Avatar.Group>
                        </Group>
                        <Stack gap={8}>
                          {agents.slice(0, 3).map((agent) => (
                            <Card key={agent.id} padding="sm" radius="md" withBorder className="mw-agent-card">
                              <Group justify="space-between" wrap="nowrap" align="flex-start">
                                <Group gap={9} wrap="nowrap" className="mw-agent-copy">
                                  <span className={`mw-agent-presence ${agent.status.toLowerCase()}`} />
                                  <div>
                                    <Text size="xs" fw={700}>{agent.name}</Text>
                                    <Text size="xs" c="dimmed" lineClamp={1}>{agent.role}</Text>
                                  </div>
                                </Group>
                                <Text size="xs" c="dimmed">{agent.progress}%</Text>
                              </Group>
                            </Card>
                          ))}
                        </Stack>
                      </section>

                      <Divider />

                      <section>
                        <Group justify="space-between" mb={9}><Text size="xs" fw={800} tt="uppercase" className="mw-section-label">Context</Text><Text size="xs" fw={700}>{detail.context}%</Text></Group>
                        <Progress value={detail.context} color="violet" radius="xl" size={7} />
                        <Text size="xs" c="dimmed" mt={8}>153k of 372k tokens · cache healthy</Text>
                      </section>
                    </Stack>
                  </ScrollArea>
                </aside>
              )}
            </div>
          </Tabs.Panel>

          <Tabs.Panel value="changes" className="mw-tab-panel">
            <div className="mw-changes-layout">
              <aside className="mw-change-list">
                <div className="mw-change-list-head">
                  <div><Text fw={750} size="sm">Review changes</Text><Text c="dimmed" size="xs">3 files · +604 −12</Text></div>
                  <Badge color={changesApproved ? "teal" : "yellow"} variant="light">{changesApproved ? "Approved" : "Pending"}</Badge>
                </div>
                {changes.map((change, index) => (
                  <button className={`mw-change-row ${index === 0 ? "selected" : ""}`} key={change.file} type="button">
                    <FileCode2 size={15} />
                    <span><strong>{change.file.split("/").at(-1)}</strong><small>{change.file}</small></span>
                    <em>{change.delta}</em>
                  </button>
                ))}
                <div className="mw-review-actions">
                  <Button fullWidth variant="default" onClick={() => setChangesApproved(false)}>Request changes</Button>
                  <Button fullWidth color="teal" leftSection={<CheckCircle2 size={15} />} onClick={() => setChangesApproved(true)}>{changesApproved ? "Changes approved" : "Approve all"}</Button>
                </div>
              </aside>
              <section className="mw-diff" aria-label="Code change preview">
                <header><span>DesignLabApp.tsx</span><span>+186 −4</span></header>
                <ScrollArea className="mw-diff-scroll" type="auto">
                  <pre aria-label="Unified code diff"><code>
                    <span className="mw-diff-context">  import {`{`} useState {`}`} from &quot;react&quot;;</span>{"\n"}
                    <span className="mw-diff-add">+ import {`{`} WorkbenchShell {`}`} from &quot;./WorkbenchShell&quot;;</span>{"\n"}
                    <span className="mw-diff-context">  </span>{"\n"}
                    <span className="mw-diff-context">  export function DesignLabApp() {`{`}</span>{"\n"}
                    <span className="mw-diff-remove">-   return &lt;Placeholder /&gt;;</span>{"\n"}
                    <span className="mw-diff-add">+   const [surface, setSurface] = useState(&quot;conversation&quot;);</span>{"\n"}
                    <span className="mw-diff-add">+   return (</span>{"\n"}
                    <span className="mw-diff-add">+     &lt;WorkbenchShell surface={`{`}surface{`}`} onSurfaceChange={`{`}setSurface{`}`}&gt;</span>{"\n"}
                    <span className="mw-diff-add">+       &lt;TaskConversation task={`{`}activeTask{`}`} /&gt;</span>{"\n"}
                    <span className="mw-diff-add">+     &lt;/WorkbenchShell&gt;</span>{"\n"}
                    <span className="mw-diff-add">+   );</span>{"\n"}
                    <span className="mw-diff-context">  {`}`}</span>
                  </code></pre>
                </ScrollArea>
              </section>
            </div>
          </Tabs.Panel>

          <Tabs.Panel value="preview" className="mw-tab-panel">
            <div className="mw-preview-stage">
              <div className="mw-preview-toolbar">
                <Group gap={8}><span className="mw-browser-dots"><i /><i /><i /></span><Text size="xs" c="dimmed">http://127.0.0.1:5173/design-lab</Text></Group>
                <SegmentedControl size="xs" value={previewSize} onChange={setPreviewSize} data={[{ label: "Desktop", value: "desktop" }, { label: "Mobile", value: "mobile" }]} aria-label="Preview viewport" />
              </div>
              <div className="mw-preview-canvas">
                <div className={`mw-preview-app ${previewSize}`}>
                  <div className="mw-preview-app-nav"><span><Sparkles size={14} />Dire</span><span>Projects&nbsp;&nbsp; Agents&nbsp;&nbsp; Settings</span></div>
                  <div className="mw-preview-hero">
                    <Badge color="violet" variant="light">Agent workspace</Badge>
                    <h2>Work is easier to trust<br />when it is easier to see.</h2>
                    <p>Track the plan, inspect every change, and steer parallel agents from one calm workspace.</p>
                    <button type="button">Open workbench <ArrowUp size={14} /></button>
                  </div>
                  <div className="mw-preview-cards"><span /><span /><span /></div>
                </div>
              </div>
            </div>
          </Tabs.Panel>
        </Tabs>
      </section>
    </div>
  );
}

function TaskGroup(props: {
  label: string;
  items: { project: (typeof projects)[number]; index: number }[];
  selectedTask: number;
  onSelect: (index: number) => void;
}) {
  return (
    <section className="mw-task-group">
      <div className="mw-task-group-label"><span>{props.label}</span><small>{props.items.length}</small></div>
      {props.items.map(({ project, index }) => {
        const tone = statusTone[project.status];
        return (
          <button
            className={`mw-task-row ${props.selectedTask === index ? "selected" : ""}`}
            key={project.name}
            type="button"
            onClick={() => props.onSelect(index)}
            aria-current={props.selectedTask === index ? "page" : undefined}
          >
            <span className={`mw-task-status ${project.status}`} />
            <span className="mw-task-copy"><strong>{project.name}</strong><small>{project.meta}</small></span>
            {project.status === "review" && <span className="mw-review-pip" aria-label={tone.label}>!</span>}
          </button>
        );
      })}
    </section>
  );
}

export function MantineWorkbench() {
  return (
    <MantineProvider theme={workbenchTheme} defaultColorScheme="light">
      <MantineWorkbenchInner />
    </MantineProvider>
  );
}

export default MantineWorkbench;
