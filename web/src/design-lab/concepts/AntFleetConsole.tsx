import {
  Avatar,
  Badge,
  Button,
  Card,
  ConfigProvider,
  Drawer,
  Input,
  Progress,
  Segmented,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  Timeline,
  Tooltip,
  Typography,
  theme as antTheme,
} from "antd";
import type { TableColumnsType } from "antd";
import {
  Activity,
  AppWindow,
  ArrowRight,
  Bot,
  Check,
  CheckCircle2,
  CirclePause,
  Clock3,
  Code2,
  Command,
  ExternalLink,
  FileCode2,
  GitBranch,
  GitPullRequest,
  LayoutDashboard,
  ListFilter,
  Menu,
  MessageSquareText,
  MoreHorizontal,
  Play,
  Radio,
  Search,
  Send,
  Server,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  Users,
  XCircle,
} from "lucide-react";
import { useMemo, useState } from "react";
import { activity, agents, changes, projects, steps } from "../mockData";
import "./AntFleetConsole.css";

const { Text, Title } = Typography;

type ProjectRow = (typeof projects)[number];
type AgentRow = (typeof agents)[number];
type ChangeRow = (typeof changes)[number];
type LifecycleFilter = "all" | ProjectRow["status"];

const lifecycleConfig = {
  running: { badge: "processing", label: "Running", color: "blue" },
  review: { badge: "warning", label: "Needs review", color: "gold" },
  idle: { badge: "default", label: "Idle", color: "default" },
  done: { badge: "success", label: "Completed", color: "green" },
} as const;

const detailByTask = [
  { id: "TASK-184", branch: "codex/design-lab", elapsed: "18m", context: 41 },
  { id: "TASK-179", branch: "fix/checkout-retry", elapsed: "43m", context: 73 },
  { id: "TASK-166", branch: "docs/navigation", elapsed: "2h", context: 26 },
  { id: "TASK-152", branch: "chore/telemetry", elapsed: "1d", context: 18 },
] as const;

const consoleTheme = {
  algorithm: antTheme.darkAlgorithm,
  token: {
    colorPrimary: "#4f8cff",
    colorInfo: "#4f8cff",
    colorSuccess: "#43c49b",
    colorWarning: "#f2ba5b",
    colorError: "#f06d78",
    colorBgBase: "#07101d",
    colorBgContainer: "#0e1928",
    colorBgElevated: "#132033",
    colorBorder: "#233249",
    colorBorderSecondary: "#1c2a3e",
    colorText: "#e9f0fa",
    colorTextSecondary: "#8fa2bb",
    borderRadius: 8,
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, sans-serif",
  },
  components: {
    Card: { headerBg: "transparent" },
    Table: { headerBg: "#101d2d", rowHoverBg: "#122239" },
    Tabs: { itemColor: "#879ab3", itemSelectedColor: "#dce8fa", inkBarColor: "#4f8cff" },
    Segmented: { itemSelectedBg: "#243a5a", trackBg: "#0a1422" },
  },
};

function AntFleetConsoleInner() {
  const [selectedTask, setSelectedTask] = useState(0);
  const [filter, setFilter] = useState<LifecycleFilter>("all");
  const [search, setSearch] = useState("");
  const [activeView, setActiveView] = useState("overview");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [paused, setPaused] = useState(false);
  const [approved, setApproved] = useState(false);
  const [steerText, setSteerText] = useState("");
  const [lastSteer, setLastSteer] = useState("");
  const selected = projects[selectedTask];
  const detail = detailByTask[selectedTask];
  const selectedLifecycle = lifecycleConfig[selected.status];

  const visibleTasks = useMemo(() => {
    const query = search.trim().toLowerCase();
    return projects
      .map((project, index) => ({ project, index }))
      .filter(({ project }) => filter === "all" || project.status === filter)
      .filter(({ project }) => !query || `${project.name} ${project.meta}`.toLowerCase().includes(query));
  }, [filter, search]);

  const selectTask = (index: number) => {
    setSelectedTask(index);
    setDrawerOpen(false);
    setPaused(false);
    setApproved(projects[index].status === "done");
  };

  const submitSteer = () => {
    const value = steerText.trim();
    if (!value) return;
    setLastSteer(value);
    setSteerText("");
  };

  const agentColumns: TableColumnsType<AgentRow> = [
    {
      title: "Agent",
      dataIndex: "name",
      key: "name",
      render: (name: AgentRow["name"], row) => (
        <Space size={9}>
          <Avatar size={27} className={`af-agent-avatar ${row.status.toLowerCase()}`}>{name.slice(0, 1).toUpperCase()}</Avatar>
          <span className="af-table-person"><strong>{name}</strong><small>{row.role}</small></span>
        </Space>
      ),
    },
    {
      title: "State",
      dataIndex: "status",
      key: "status",
      width: 108,
      render: (status: AgentRow["status"]) => (
        <Badge
          status={status === "Done" ? "success" : status === "Blocked" ? "warning" : "processing"}
          text={<span className="af-badge-text">{status}</span>}
        />
      ),
    },
    {
      title: "Progress",
      dataIndex: "progress",
      key: "progress",
      width: 126,
      render: (progress: number) => <Progress percent={progress} size="small" showInfo={false} strokeColor={progress === 100 ? "#43c49b" : "#4f8cff"} />,
    },
    {
      title: "",
      key: "actions",
      width: 34,
      align: "right",
      render: () => <Button type="text" size="small" icon={<MoreHorizontal size={15} />} aria-label="Agent actions" />,
    },
  ];

  const changeColumns: TableColumnsType<ChangeRow> = [
    {
      title: "File",
      dataIndex: "file",
      key: "file",
      render: (file: string) => <span className="af-file-cell"><FileCode2 size={14} /><span><strong>{file.split("/").at(-1)}</strong><small>{file}</small></span></span>,
    },
    { title: "Delta", dataIndex: "delta", key: "delta", width: 82, render: (delta: string) => <Text className="af-delta">{delta}</Text> },
    { title: "State", dataIndex: "state", key: "state", width: 96, render: (state: string) => <Tag color="blue">{state}</Tag> },
  ];

  const navigation = (
    <div className="af-navigation">
      <div className="af-fleet-heading">
        <div><Text className="af-overline">Mission control</Text><Title level={4}>Task fleet</Title></div>
        <Button type="text" size="small" icon={<ListFilter size={15} />} aria-label="Fleet filter options" />
      </div>

      <Input
        value={search}
        onChange={(event) => setSearch(event.currentTarget.value)}
        prefix={<Search size={14} />}
        placeholder="Search tasks"
        allowClear
        aria-label="Search fleet tasks"
      />

      <Segmented
        block
        size="small"
        value={filter}
        onChange={(value) => setFilter(String(value) as LifecycleFilter)}
        options={[
          { label: "All", value: "all" },
          { label: "Live", value: "running" },
          { label: "Review", value: "review" },
        ]}
        aria-label="Filter tasks by lifecycle"
      />

      <div className="af-task-list" role="list" aria-label="Agent tasks">
        {visibleTasks.map(({ project, index }) => {
          const config = lifecycleConfig[project.status];
          return (
            <button
              key={project.name}
              className={`af-task-card ${selectedTask === index ? "selected" : ""}`}
              type="button"
              onClick={() => selectTask(index)}
              aria-current={selectedTask === index ? "page" : undefined}
            >
              <span className="af-task-card-top">
                <Badge status={config.badge} />
                <span>{detailByTask[index].id}</span>
                <small>{detailByTask[index].elapsed}</small>
              </span>
              <strong>{project.name}</strong>
              <span className="af-task-card-bottom"><small>{project.meta}</small><ArrowRight size={13} /></span>
            </button>
          );
        })}
        {visibleTasks.length === 0 && <div className="af-no-results">No matching tasks</div>}
      </div>

      <div className="af-fleet-health">
        <div className="af-health-title"><span><Server size={14} />Fleet health</span><Tag color="green">Healthy</Tag></div>
        <div className="af-health-stats"><span><strong>2</strong> active</span><span><strong>1</strong> waiting</span><span><strong>0</strong> failed</span></div>
      </div>
    </div>
  );

  const overviewPanel = (
    <div className="af-overview-grid">
      <Card className="af-activity-card" title={<span className="af-card-title"><Radio size={15} />Live activity</span>} extra={<Button type="link" size="small" onClick={() => setActiveView("activity")}>View log</Button>}>
        <Timeline
          className="af-timeline"
          items={[
            ...(lastSteer ? [{ color: "#8eaefc", content: <ActivityItem time="now" agent="you" text={lastSteer} tone="steer" /> }] : []),
            ...activity.map((event) => ({
              color: event.tone === "done" ? "#43c49b" : event.tone === "checkpoint" ? "#f2ba5b" : "#4f8cff",
              content: <ActivityItem time={event.time} agent={event.agent} text={event.text} tone={event.tone} />,
            })),
          ]}
        />
        <div className="af-steer-box">
          <Input
            value={steerText}
            onChange={(event) => setSteerText(event.currentTarget.value)}
            onPressEnter={submitSteer}
            placeholder={paused ? "Resume the task to send guidance" : "Steer the active task…"}
            disabled={paused || selected.status === "done"}
            suffix={<Button type="text" size="small" icon={<Send size={14} />} onClick={submitSteer} disabled={!steerText.trim()} aria-label="Send steering message" />}
            aria-label="Steer the active task"
          />
          <span>Delivered after the current tool call</span>
        </div>
      </Card>

      <Card className="af-agents-card" title={<span className="af-card-title"><Users size={15} />Agent team</span>} extra={<Tag color="blue">3 of 4 live</Tag>}>
        <Table<AgentRow>
          className="af-agent-table"
          columns={agentColumns}
          dataSource={[...agents]}
          rowKey="id"
          pagination={false}
          size="small"
          scroll={{ x: 440 }}
        />
      </Card>

      <Card className="af-plan-card" title={<span className="af-card-title"><CheckCircle2 size={15} />Execution plan</span>} extra={<Text type="secondary">2 / 4</Text>}>
        <div className="af-plan-list">
          {steps.map((step, index) => (
            <div className={`af-plan-step ${step.state}`} key={step.label}>
              <span>{step.state === "done" ? <Check size={12} /> : index + 1}</span>
              <div><Text>{step.label}</Text>{step.state === "active" && <small>ui-builder · running now</small>}</div>
            </div>
          ))}
        </div>
      </Card>

      <Card className="af-review-card" title={<span className="af-card-title"><GitPullRequest size={15} />Review gate</span>} extra={<Badge status={approved ? "success" : "warning"} text={approved ? "Approved" : "Action needed"} />}>
        <div className="af-review-summary">
          <div className="af-review-icon"><Code2 size={20} /></div>
          <div><Text strong>3 files are ready</Text><Text type="secondary">+604 −12 · tests passing</Text></div>
          <Button size="small" type={approved ? "default" : "primary"} onClick={() => setActiveView("review")}>{approved ? "View diff" : "Review changes"}</Button>
        </div>
        <div className="af-check-row"><span><CheckCircle2 size={14} />Typecheck</span><Tag color="green">Passed</Tag></div>
        <div className="af-check-row"><span><CheckCircle2 size={14} />Component tests</span><Tag color="green">18 passed</Tag></div>
      </Card>

      <Card className="af-preview-card" title={<span className="af-card-title"><AppWindow size={15} />Live preview</span>} extra={<Button type="text" size="small" icon={<ExternalLink size={14} />} aria-label="Open preview in new window" />}>
        <div className="af-preview-window">
          <div className="af-preview-browser"><span><i /><i /><i /></span><small>127.0.0.1:5173</small></div>
          <div className="af-preview-body">
            <span className="af-preview-kicker">DIRE WORKBENCH</span>
            <strong>Parallel work,<br />one clear view.</strong>
            <span className="af-preview-bars"><i /><i /><i /></span>
          </div>
        </div>
      </Card>
    </div>
  );

  const activityPanel = (
    <div className="af-activity-view">
      <Card title={<span className="af-card-title"><Activity size={15} />Full session log</span>} extra={<Space><Badge status="processing" text="Live" /><Button size="small">Export</Button></Space>}>
        <Timeline
          mode="left"
          className="af-long-timeline"
          items={[
            { color: "#4f8cff", label: "10:42", content: <LogEntry title="Responsive shell created" agent="ui-builder" detail="Wrote the workspace grid, task navigation, and breakpoint behavior." tags={["write", "3 files"]} /> },
            { color: "#43c49b", label: "10:39", content: <LogEntry title="UX review completed" agent="ux-reviewer" detail="Recommended a task-first hierarchy with review and preview as first-class surfaces." tags={["research", "complete"]} /> },
            { color: "#f2ba5b", label: "10:34", content: <LogEntry title="Checkpoint saved" agent="orchestrator" detail="Captured the workspace before adding component-library dependencies." tags={["checkpoint", "safe to restore"]} /> },
            { color: "#4f8cff", label: "10:31", content: <LogEntry title="Task accepted" agent="orchestrator" detail="Split discovery, implementation, and verification across the agent team." tags={["plan", "4 steps"]} /> },
          ]}
        />
      </Card>
      <Card className="af-queue-card" title="Instruction queue" extra={<Tag color="blue">1 queued</Tag>}>
        <div className="af-queue-row"><Clock3 size={15} /><span><strong>Validate keyboard and mobile flows</strong><small>Runs after the current implementation step</small></span><Button type="text" size="small" icon={<XCircle size={15} />} aria-label="Remove queued instruction" /></div>
        <Input.Search placeholder="Queue a follow-up…" enterButton="Queue" aria-label="Queue a follow-up instruction" />
      </Card>
    </div>
  );

  const reviewPanel = (
    <div className="af-review-view">
      <Card className="af-files-card" title={<span className="af-card-title"><FileCode2 size={15} />Changed files</span>} extra={<Text type="secondary">+604 −12</Text>}>
        <Table<ChangeRow> columns={changeColumns} dataSource={[...changes]} rowKey="file" pagination={false} size="small" />
        <div className="af-review-buttons">
          <Button onClick={() => setApproved(false)}>Request changes</Button>
          <Button type="primary" icon={<Check size={15} />} onClick={() => setApproved(true)}>{approved ? "Approved" : "Approve changes"}</Button>
        </div>
      </Card>
      <Card className="af-code-card" title="DesignLabApp.tsx" extra={<Tag color="green">+186</Tag>}>
        <pre aria-label="Code review diff"><code>
          <span className="af-code-muted">  export function DesignLabApp() {`{`}</span>{"\n"}
          <span className="af-code-remove">-   return &lt;EmptyState /&gt;;</span>{"\n"}
          <span className="af-code-add">+   return &lt;FleetConsole task={`{`}activeTask{`}`} /&gt;;</span>{"\n"}
          <span className="af-code-add">+   // Lifecycle, review, and preview stay visible.</span>{"\n"}
          <span className="af-code-muted">  {`}`}</span>
        </code></pre>
        <div className="af-inline-comment"><Avatar size={25}>IM</Avatar><span><strong>Review note</strong><small>The task state remains visible when moving between review surfaces.</small></span></div>
      </Card>
    </div>
  );

  return (
    <div className="af-root">
      <header className="af-global-header">
        <div className="af-brand">
          <Button className="af-mobile-menu" type="text" icon={<Menu size={18} />} onClick={() => setDrawerOpen(true)} aria-label="Open fleet navigation" />
          <span className="af-brand-mark"><Command size={17} /></span>
          <span><strong>Dire</strong><small>Fleet Console</small></span>
        </div>
        <label className="af-command-search">
          <Search size={14} />
          <input placeholder="Search tasks, agents, files…" aria-label="Global search" />
          <kbd>⌘ K</kbd>
        </label>
        <Space size={8} className="af-global-actions">
          <Tooltip title="Terminal"><Button type="text" icon={<TerminalSquare size={16} />} aria-label="Open terminal" /></Tooltip>
          <Tooltip title="Connection healthy"><Button type="text" icon={<Server size={16} />} aria-label="Connection healthy" /></Tooltip>
          <Button type="primary" icon={<Sparkles size={15} />}><span className="af-button-label">New task</span></Button>
          <Avatar size={30}>IM</Avatar>
        </Space>
      </header>

      <div className="af-layout">
        <aside className="af-desktop-nav" aria-label="Fleet navigation">{navigation}</aside>
        <main className="af-main">
          <section className="af-task-header">
            <div className="af-task-heading">
              <Space size={8} wrap>
                <Badge status={paused ? "default" : selectedLifecycle.badge} />
                <Text className="af-task-id">{detail.id}</Text>
                <Text type="secondary">/</Text>
                <Text type="secondary">Dire Agent</Text>
              </Space>
              <Title level={2}>{selected.name}</Title>
              <Space size={14} wrap className="af-task-meta">
                <span><GitBranch size={13} />{detail.branch}</span>
                <span><ShieldCheck size={13} />Workspace sandbox</span>
                <span><Clock3 size={13} />{detail.elapsed} elapsed</span>
              </Space>
            </div>
            <Space size={8} className="af-task-actions">
              <Tag color={paused ? "default" : selectedLifecycle.color}>{paused ? "Paused" : selectedLifecycle.label}</Tag>
              <Button icon={paused ? <Play size={15} /> : <CirclePause size={15} />} onClick={() => setPaused((value) => !value)}>{paused ? "Resume" : "Pause"}</Button>
              <Button type="primary" icon={<MessageSquareText size={15} />} onClick={() => setActiveView("activity")}><span className="af-button-label">Steer</span></Button>
              <Button type="text" icon={<MoreHorizontal size={16} />} aria-label="More task actions" />
            </Space>
          </section>

          <section className="af-metrics" aria-label="Task metrics">
            <Card size="small"><Statistic title="Active agents" value={3} suffix="/ 4" prefix={<Bot size={16} />} /></Card>
            <Card size="small"><Statistic title="Files changed" value={3} prefix={<FileCode2 size={16} />} /></Card>
            <Card size="small"><Statistic title="Context used" value={detail.context} suffix="%" prefix={<Activity size={16} />} /></Card>
            <Card size="small"><Statistic title="Review gates" value={approved ? 0 : 1} suffix="open" prefix={<GitPullRequest size={16} />} /></Card>
          </section>

          <Tabs
            className="af-tabs"
            activeKey={activeView}
            onChange={setActiveView}
            items={[
              { key: "overview", label: <span className="af-tab-label"><LayoutDashboard size={14} />Overview</span>, children: overviewPanel },
              { key: "activity", label: <span className="af-tab-label"><Activity size={14} />Activity</span>, children: activityPanel },
              { key: "review", label: <span className="af-tab-label"><Code2 size={14} />Review <Badge count={approved ? 0 : 3} size="small" /></span>, children: reviewPanel },
            ]}
          />
        </main>
      </div>

      <Drawer
        className="af-mobile-drawer"
        title="Fleet navigation"
        placement="left"
        size={286}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        styles={{ body: { padding: 0 } }}
      >
        {navigation}
      </Drawer>
    </div>
  );
}

function ActivityItem(props: { time: string; agent: string; text: string; tone: string }) {
  return (
    <div className="af-activity-item">
      <span><strong>{props.agent}</strong><small>{props.time}</small></span>
      <Text>{props.text}</Text>
      {props.tone === "checkpoint" && <Tag color="gold">Restore point</Tag>}
    </div>
  );
}

function LogEntry(props: { title: string; agent: string; detail: string; tags: string[] }) {
  return (
    <div className="af-log-entry">
      <Text strong>{props.title}</Text>
      <Text type="secondary">{props.agent}</Text>
      <p>{props.detail}</p>
      <Space size={5} wrap>{props.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}</Space>
    </div>
  );
}

export function AntFleetConsole() {
  return (
    <ConfigProvider theme={consoleTheme}>
      <AntFleetConsoleInner />
    </ConfigProvider>
  );
}

export default AntFleetConsole;
