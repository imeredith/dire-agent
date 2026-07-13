import {
  FolderKanban,
  BookOpenText,
  CalendarClock,
  MessageSquareText,
  Plus,
  Search,
  Settings2,
  TerminalSquare,
  Trash2,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { compactEndpoint, compactPath, relativeTime } from "../lib/display";
import type { ConnectionStatus, Conversation } from "../lib/protocol";
import { readAppStorage, removeAppStorage, writeAppStorage } from "../lib/storage";

export type AppView = "conversation" | "schedules" | "settings";

interface SidebarProps {
  open: boolean;
  endpoint: string;
  connection: ConnectionStatus;
  chats: Conversation[];
  projects: Conversation[];
  selectedID: string;
  view: AppView;
  onClose: () => void;
  onSelect: (conversation: Conversation) => void;
  onSettings: () => void;
  onSchedules: () => void;
  onCreateChat: () => void;
  onCreateProject: () => void;
  onDelete: (conversation: Conversation) => void;
  onConnection: () => void;
}

export function AppSidebar(props: SidebarProps) {
  const [search, setSearch] = useState("");
  const [categoryFilter, setCategoryFilter] = useState(() =>
    readAppStorage("project.categoryFilter") || ALL_CATEGORIES);
  const filter = search.trim().toLowerCase();
  const chats = useMemo(() => filterResources(props.chats, filter), [props.chats, filter]);
  const categories = useMemo(() => projectCategories(props.projects), [props.projects]);
  const categoryProjects = useMemo(() => categoryFilter === ALL_CATEGORIES
    ? props.projects
    : props.projects.filter((project) => projectCategoryKey(project.category) === categoryFilter),
  [categoryFilter, props.projects]);
  const projects = useMemo(() => filterResources(categoryProjects, filter), [categoryProjects, filter]);
  const projectGroups = useMemo(() => groupProjects(projects), [projects]);

  useEffect(() => {
    if (props.projects.length > 0 && categoryFilter !== ALL_CATEGORIES && !categories.some((category) => category.key === categoryFilter)) {
      setCategoryFilter(ALL_CATEGORIES);
      writeAppStorage("project.categoryFilter", ALL_CATEGORIES);
      removeAppStorage("project.category");
    }
  }, [categories, categoryFilter, props.projects.length]);

  useEffect(() => {
    if (categoryFilter === ALL_CATEGORIES || categoryProjects.length === 0) return;
    const selectedProject = props.projects.find((project) => project.id === props.selectedID);
    if (selectedProject && !categoryProjects.some((project) => project.id === selectedProject.id)) {
      props.onSelect(categoryProjects[0]);
    }
  }, [categoryFilter, categoryProjects, props.onSelect, props.projects, props.selectedID]);

  const selectCategory = (value: string) => {
    setCategoryFilter(value);
    writeAppStorage("project.categoryFilter", value);
    const category = categories.find((item) => item.key === value)?.value;
    if (category) writeAppStorage("project.category", category);
    else removeAppStorage("project.category");
    if (value !== ALL_CATEGORIES) {
      const visible = props.projects.filter((project) => projectCategoryKey(project.category) === value);
      const selectedProject = props.projects.find((project) => project.id === props.selectedID);
      if (selectedProject && !visible.some((project) => project.id === selectedProject.id) && visible[0]) {
        props.onSelect(visible[0]);
      }
    }
  };

  return (
    <>
      {props.open && <button className="sidebar-scrim fixed inset-0 z-[65] bg-black/55 lg:hidden" onClick={props.onClose} aria-label="Close navigation" />}
      <aside className={`app-sidebar flex h-full min-h-0 flex-col overflow-hidden border-r border-white/10 bg-panel px-3 pt-3.5 pb-3 ${props.open ? "open" : ""}`} aria-label="Workspace navigation">
        <div className="sidebar-brand flex min-h-10 items-center gap-2.5 px-1 pb-3">
          <div className="brand-mark"><TerminalSquare size={19} /></div>
          <div><strong>Dire Agent</strong><span>agent control plane</span></div>
          <button className="icon-button sidebar-close" onClick={props.onClose} aria-label="Close navigation"><X size={17} /></button>
        </div>

        <label className="search-box flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 text-slate-600 focus-within:border-white/20 focus-within:text-slate-400">
          <Search size={15} />
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search conversations"
            aria-label="Search conversations"
          />
        </label>

        <nav className="sidebar-navigation min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
          <ResourceSection
            title="Chats"
            icon={<MessageSquareText size={14} />}
            resources={chats}
            selectedID={props.view === "conversation" ? props.selectedID : ""}
            empty={filter ? "No matching chats" : "No standalone chats"}
            onCreate={props.onCreateChat}
            onSelect={props.onSelect}
            onDelete={props.onDelete}
          />
          <ProjectSection
            groups={projectGroups}
            total={projects.length}
            categories={categories}
            categoryFilter={categoryFilter}
            selectedID={props.view === "conversation" ? props.selectedID : ""}
            empty={filter || categoryFilter !== ALL_CATEGORIES ? "No matching projects" : "No projects yet"}
            onCategoryChange={selectCategory}
            onCreate={props.onCreateProject}
            onSelect={props.onSelect}
            onDelete={props.onDelete}
          />
        </nav>

        <div className="sidebar-bottom grid shrink-0 gap-1 border-t border-white/[0.08] pt-2.5">
          <a href="/docs" className="group flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left text-slate-400 transition hover:bg-white/5 hover:text-slate-100">
            <BookOpenText size={15} />
            <span className="grid gap-0.5"><strong className="text-[10px] text-slate-300">Documentation</strong><small className="text-[8px] text-slate-600">Feature guides & browser tests</small></span>
          </a>
          <button aria-label="Scheduled prompts" className={`settings-nav ${props.view === "schedules" ? "selected" : ""}`} onClick={props.onSchedules}>
            <CalendarClock size={15} />
            <span><strong>Scheduled prompts</strong><small>Recurring and one-off tasks</small></span>
          </button>
          <button aria-label="Settings" className={`settings-nav ${props.view === "settings" ? "selected" : ""}`} onClick={props.onSettings}>
            <Settings2 size={15} />
            <span><strong>Settings</strong><small>Models, skills, MCP & agents</small></span>
          </button>
          <button
            className="connection-row"
            onClick={props.onConnection}
            aria-label={props.connection === "online" ? "Daemon online" : props.connection}
          >
            <span className={`connection-dot ${props.connection}`} />
            <span><strong>{props.connection === "online" ? "Daemon online" : props.connection}</strong><small>{compactEndpoint(props.endpoint)}</small></span>
          </button>
        </div>
      </aside>
    </>
  );
}

const ALL_CATEGORIES = "__all_categories__";
const UNCATEGORIZED = "__uncategorized__";

interface ProjectCategory {
  key: string;
  label: string;
  value: string;
}

interface ProjectGroup extends ProjectCategory {
  projects: Conversation[];
}

function ProjectSection(props: {
  groups: ProjectGroup[];
  total: number;
  categories: ProjectCategory[];
  categoryFilter: string;
  selectedID: string;
  empty: string;
  onCategoryChange: (value: string) => void;
  onCreate: () => void;
  onSelect: (resource: Conversation) => void;
  onDelete: (resource: Conversation) => void;
}) {
  return (
    <section className="resource-section project-section">
      <div className="resource-section-heading">
        <span><FolderKanban size={14} />Projects<small>{props.total}</small></span>
        <button onClick={props.onCreate} aria-label="New project"><Plus size={14} /></button>
      </div>
      <label className="category-filter">
        <span>Visible category</span>
        <select
          aria-label="Project category filter"
          value={props.categoryFilter}
          onChange={(event) => props.onCategoryChange(event.target.value)}
        >
          <option value={ALL_CATEGORIES}>All categories</option>
          {props.categories.map((category) => (
            <option key={category.key} value={category.key}>{category.label}</option>
          ))}
        </select>
      </label>
      <div className="resource-list" aria-label="Projects">
        {props.groups.map((group) => (
          <section className="project-category-group" key={group.key} aria-label={`${group.label} projects`}>
            <div className="project-category-heading"><span>{group.label}</span><small>{group.projects.length}</small></div>
            {group.projects.map((resource) => (
              <ResourceRow
                key={resource.id}
                resource={resource}
                selected={props.selectedID === resource.id}
                onSelect={props.onSelect}
                onDelete={props.onDelete}
              />
            ))}
          </section>
        ))}
        {!props.groups.length && <p className="resource-empty">{props.empty}</p>}
      </div>
    </section>
  );
}

function ResourceSection(props: {
  title: string;
  icon: React.ReactNode;
  resources: Conversation[];
  selectedID: string;
  empty: string;
  onCreate: () => void;
  onSelect: (resource: Conversation) => void;
  onDelete: (resource: Conversation) => void;
}) {
  return (
    <section className="resource-section">
      <div className="resource-section-heading">
        <span>{props.icon}{props.title}<small>{props.resources.length}</small></span>
        <button onClick={props.onCreate} aria-label={`New ${props.title === "Chats" ? "chat" : "project"}`}><Plus size={14} /></button>
      </div>
      <div className="resource-list" aria-label={props.title}>
        {props.resources.map((resource) => (
          <ResourceRow
            key={resource.id}
            resource={resource}
            selected={props.selectedID === resource.id}
            onSelect={props.onSelect}
            onDelete={props.onDelete}
          />
        ))}
        {!props.resources.length && <p className="resource-empty">{props.empty}</p>}
      </div>
    </section>
  );
}

function ResourceRow(props: {
  resource: Conversation;
  selected: boolean;
  onSelect: (resource: Conversation) => void;
  onDelete: (resource: Conversation) => void;
}) {
  const { resource } = props;
  return (
    <div className={`resource-row ${props.selected ? "selected" : ""}`}>
      <button className="resource-select" onClick={() => props.onSelect(resource)}>
        <span className={`resource-status ${resource.status === "running" ? "running" : ""}`} />
        <span>
          <strong>{resource.name || (resource.id.startsWith("chat_") ? "Untitled chat" : "Unnamed project")}</strong>
          <small>
            {resource.worktree ? "Worktree · " : ""}
            {resource.cwd ? `${compactPath(resource.cwd)} · ` : ""}{resource.model} · {relativeTime(resource.updated_at)}
          </small>
        </span>
      </button>
      <button className="row-delete" onClick={() => props.onDelete(resource)} aria-label={`Delete ${resource.name || resource.id}`}>
        <Trash2 size={13} />
      </button>
    </div>
  );
}

function projectCategoryKey(category: string | undefined): string {
  const value = category?.trim() || "";
  return value ? `category:${value}` : UNCATEGORIZED;
}

function projectCategories(projects: Conversation[]): ProjectCategory[] {
  const values = new Map<string, ProjectCategory>();
  for (const project of projects) {
    const value = project.category?.trim() || "";
    const key = projectCategoryKey(value);
    values.set(key, { key, value, label: value || "Uncategorized" });
  }
  return [...values.values()].sort((left, right) => left.label.localeCompare(right.label));
}

function groupProjects(projects: Conversation[]): ProjectGroup[] {
  const groups = new Map<string, ProjectGroup>();
  for (const project of projects) {
    const value = project.category?.trim() || "";
    const key = projectCategoryKey(value);
    const group = groups.get(key) || { key, value, label: value || "Uncategorized", projects: [] };
    group.projects.push(project);
    groups.set(key, group);
  }
  return [...groups.values()].sort((left, right) => left.label.localeCompare(right.label));
}

function filterResources(resources: Conversation[], query: string): Conversation[] {
  if (!query) return resources;
  return resources.filter((item) =>
    item.name?.toLowerCase().includes(query) ||
    item.id.toLowerCase().includes(query) ||
    item.model.toLowerCase().includes(query) ||
    item.category?.toLowerCase().includes(query) ||
    item.cwd?.toLowerCase().includes(query) ||
    item.worktree?.source_cwd?.toLowerCase().includes(query) ||
    item.worktree?.source_repository?.toLowerCase().includes(query) ||
    item.worktree?.base_ref?.toLowerCase().includes(query) ||
    item.worktree?.environment_id?.toLowerCase().includes(query));
}
