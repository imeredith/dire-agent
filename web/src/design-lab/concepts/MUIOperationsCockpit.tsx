import {
  Activity,
  Bell,
  Bot,
  CheckCircle2,
  ChevronRight,
  CirclePause,
  CirclePlay,
  Code2,
  Command,
  FileCode2,
  GitBranch,
  LayoutDashboard,
  MessageSquareText,
  MoreHorizontal,
  Play,
  Search,
  Send,
  Settings2,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  UsersRound,
} from "lucide-react";
import {
  Avatar,
  Badge as MuiBadge,
  Box,
  Button,
  Chip,
  Divider,
  IconButton,
  InputAdornment,
  LinearProgress,
  List,
  ListItemAvatar,
  ListItemButton,
  ListItemText,
  Paper,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { alpha, createTheme, ThemeProvider } from "@mui/material/styles";
import { FormEvent, ReactNode, useMemo, useState } from "react";
import { activity, agents, changes, projects, steps } from "../mockData";

const muiCockpitTheme = createTheme({
  palette: {
    mode: "dark",
    primary: { main: "#7c9cff", dark: "#5475df", light: "#b7c7ff", contrastText: "#071021" },
    secondary: { main: "#bb9cff", light: "#d5c5ff" },
    success: { main: "#52d6ad", light: "#8de8cc" },
    warning: { main: "#f3b963", light: "#ffd394" },
    background: { default: "#090f1c", paper: "#111a2a" },
    text: { primary: "#eef3ff", secondary: "#94a3be" },
    divider: "#26334a",
  },
  typography: {
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
    button: { textTransform: "none", fontWeight: 700 },
  },
  shape: { borderRadius: 12 },
  components: {
    MuiButton: { styleOverrides: { root: { boxShadow: "none", borderRadius: 9 } } },
    MuiPaper: { styleOverrides: { root: { backgroundImage: "none" } } },
    MuiChip: { styleOverrides: { root: { fontWeight: 700 } } },
    MuiTab: { styleOverrides: { root: { textTransform: "none", fontWeight: 700, minHeight: 42 } } },
    MuiOutlinedInput: {
      styleOverrides: {
        root: {
          backgroundColor: "#0d1524",
          "&:hover .MuiOutlinedInput-notchedOutline": { borderColor: "#445574" },
        },
      },
    },
  },
});

const statusColors: Record<string, { color: string; bg: string }> = {
  Running: { color: "#72e2bf", bg: "#15352f" },
  Done: { color: "#a9bbff", bg: "#1b2a4f" },
  Blocked: { color: "#ffc879", bg: "#3b2a16" },
};

const cockpitNav: Array<{ icon: ReactNode; label: string; selected: boolean }> = [
  { icon: <LayoutDashboard size={14} />, label: "Overview", selected: true },
  { icon: <MessageSquareText size={14} />, label: "Conversations", selected: false },
  { icon: <UsersRound size={14} />, label: "Agent team", selected: false },
  { icon: <Code2 size={14} />, label: "Tools & terminals", selected: false },
];

const extendedActivity: Array<{ time: string; agent: string; text: string; tone: string }> = [
  ...activity,
  { time: "10:29", agent: "qa", text: "Flagged the mobile tab labels for review", tone: "checkpoint" },
  { time: "10:24", agent: "orchestrator", text: "Assigned one concept to each UI library", tone: "running" },
];

function MiniMetric(props: { icon: ReactNode; label: string; value: string; detail: string; tint: string }) {
  return (
    <Paper variant="outlined" sx={{ p: 1.6, minWidth: 0, borderColor: "divider", borderRadius: 3 }}>
      <Stack direction="row" spacing={1.2} sx={{ alignItems: "center" }}>
        <Avatar variant="rounded" sx={{ width: 34, height: 34, bgcolor: alpha(props.tint, .11), color: props.tint }}>{props.icon}</Avatar>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="caption" color="text.secondary" sx={{ fontSize: 9, fontWeight: 700, letterSpacing: ".06em", textTransform: "uppercase" }}>{props.label}</Typography>
          <Stack direction="row" spacing={.7} sx={{ alignItems: "baseline" }}><Typography sx={{ fontSize: 17, lineHeight: 1.1, fontWeight: 800, letterSpacing: "-.03em" }}>{props.value}</Typography><Typography color="text.secondary" sx={{ fontSize: 8 }}>{props.detail}</Typography></Stack>
        </Box>
      </Stack>
    </Paper>
  );
}

export function MUIOperationsCockpit() {
  const [selectedProject, setSelectedProject] = useState<string>(projects[0].name);
  const [selectedAgent, setSelectedAgent] = useState<string>(agents[0].id);
  const [view, setView] = useState(0);
  const [paused, setPaused] = useState(false);
  const [command, setCommand] = useState("");
  const [announcement, setAnnouncement] = useState("Release workspace is ready.");

  const activeAgent = useMemo(() => agents.find((agent) => agent.id === selectedAgent) ?? agents[0], [selectedAgent]);

  const submitCommand = (event: FormEvent) => {
    event.preventDefault();
    const text = command.trim();
    if (!text) return;
    setAnnouncement(`Sent to ${activeAgent.name}: ${text}`);
    setCommand("");
  };

  return (
    <ThemeProvider theme={muiCockpitTheme}>
      <Box
        component="section"
        aria-label="Material UI operations cockpit concept"
        sx={{
          width: "100%",
          height: "100%",
          minWidth: 0,
          minHeight: 0,
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          bgcolor: "background.default",
          color: "text.primary",
        }}
      >
        <Box
          component="header"
          sx={{
            height: 62,
            flex: "0 0 62px",
            display: "flex",
            alignItems: "center",
            gap: 1.5,
            px: { xs: 1.25, sm: 2 },
            bgcolor: "#070d18",
            color: "#f4f7ff",
            borderBottom: "1px solid #202c40",
            boxShadow: "0 10px 30px rgba(0,0,0,.34)",
            zIndex: 3,
          }}
        >
          <Avatar variant="rounded" sx={{ width: 34, height: 34, bgcolor: "#6888f5", color: "#071021" }}><TerminalSquare size={18} /></Avatar>
          <Box sx={{ display: { xs: "none", sm: "block" }, minWidth: 122 }}>
            <Typography sx={{ fontSize: 12, lineHeight: 1.1, fontWeight: 800, letterSpacing: "-.02em" }}>Dire Operations</Typography>
            <Typography sx={{ mt: .25, color: "#91a0bd", fontSize: 8, letterSpacing: ".06em", textTransform: "uppercase" }}>Agent workspace</Typography>
          </Box>
          <Divider orientation="vertical" flexItem sx={{ mx: .5, borderColor: "rgba(255,255,255,.12)" }} />
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography noWrap sx={{ color: "#91a0bd", fontSize: 8, fontWeight: 700, letterSpacing: ".08em", textTransform: "uppercase" }}>Current project</Typography>
            <Typography noWrap sx={{ mt: .2, fontSize: 10, fontWeight: 700 }}>{selectedProject}</Typography>
          </Box>
          <Chip size="small" icon={<ShieldCheck size={12} />} label="Workspace protected" sx={{ display: { xs: "none", md: "flex" }, height: 26, bgcolor: "rgba(82,214,173,.14)", color: "#91ebcf", border: "1px solid rgba(82,214,173,.2)", "& .MuiChip-icon": { color: "#91ebcf" }, fontSize: 8 }} />
          <Tooltip title="Search">
            <IconButton aria-label="Search workspace" sx={{ color: "#aeb8cf" }}><Search size={17} /></IconButton>
          </Tooltip>
          <Tooltip title="Notifications">
            <IconButton aria-label="Open notifications" sx={{ color: "#aeb8cf" }}><MuiBadge variant="dot" color="secondary"><Bell size={17} /></MuiBadge></IconButton>
          </Tooltip>
          <Button
            variant="contained"
            size="small"
            startIcon={paused ? <CirclePlay size={14} /> : <CirclePause size={14} />}
            onClick={() => {
              setPaused((value) => !value);
              setAnnouncement(paused ? "All agents resumed." : "All agents paused at a safe checkpoint.");
            }}
            sx={{ ml: .5, minWidth: { xs: 36, sm: 108 }, height: 32, px: { xs: 1, sm: 1.4 }, bgcolor: paused ? "#3ec39a" : "#718ff2", color: "#071021", "&:hover": { bgcolor: paused ? "#55d4ae" : "#89a3fa" }, "& .MuiButton-startIcon": { mr: { xs: 0, sm: .7 } } }}
          >
            <Box component="span" sx={{ display: { xs: "none", sm: "inline" } }}>{paused ? "Resume team" : "Pause team"}</Box>
          </Button>
        </Box>

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "minmax(0,1fr)", md: "216px minmax(0,1fr)", lg: "216px minmax(0,1fr) 292px" },
            minWidth: 0,
            minHeight: 0,
            flex: 1,
            overflow: { xs: "auto", md: "hidden" },
          }}
        >
          <Paper component="aside" square elevation={0} aria-label="Project navigation" sx={{ display: { xs: "none", md: "flex" }, minHeight: 0, flexDirection: "column", borderRight: 1, borderColor: "divider", overflow: "hidden" }}>
            <Box sx={{ px: 1.5, pt: 1.8, pb: .8 }}>
              <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center" }}>
                <Typography sx={{ color: "text.secondary", fontSize: 8, fontWeight: 800, letterSpacing: ".1em", textTransform: "uppercase" }}>Projects</Typography>
                <IconButton size="small" aria-label="Project menu"><MoreHorizontal size={15} /></IconButton>
              </Stack>
            </Box>
            <List disablePadding sx={{ px: .8 }}>
              {projects.map((project) => (
                <ListItemButton
                  key={project.name}
                  selected={selectedProject === project.name}
                  onClick={() => setSelectedProject(project.name)}
                  sx={{ mb: .35, minHeight: 51, borderRadius: 2, px: 1, "&:hover": { bgcolor: alpha("#7c9cff", .07) }, "&.Mui-selected": { bgcolor: alpha("#7c9cff", .13), color: "primary.light" }, "&.Mui-selected:hover": { bgcolor: alpha("#7c9cff", .18) } }}
                >
                  <ListItemAvatar sx={{ minWidth: 32 }}>
                    <Avatar variant="rounded" sx={{ width: 25, height: 25, bgcolor: project.status === "running" ? "#15352f" : "#1a2435", color: project.status === "running" ? "#72e2bf" : "#9aa9c2" }}><GitBranch size={12} /></Avatar>
                  </ListItemAvatar>
                  <ListItemText primary={project.name} secondary={project.meta} slotProps={{ primary: { noWrap: true, sx: { fontSize: 9, fontWeight: 750 } }, secondary: { noWrap: true, sx: { fontSize: 7, mt: .25 } } }} />
                  {project.status === "running" && <Box sx={{ width: 6, height: 6, borderRadius: 9, bgcolor: "success.main", boxShadow: "0 0 0 3px rgba(82,214,173,.12)" }} />}
                </ListItemButton>
              ))}
            </List>

            <Divider sx={{ mx: 1.5, my: 1.3 }} />
            <Box component="nav" aria-label="Workspace sections" sx={{ px: .8 }}>
              {cockpitNav.map(({ icon, label, selected }) => (
                <Button key={label} fullWidth color="inherit" startIcon={icon} sx={{ mb: .25, minHeight: 34, justifyContent: "flex-start", px: 1, color: selected ? "primary.light" : "text.secondary", bgcolor: selected ? alpha("#7c9cff", .1) : "transparent", fontSize: 8.5 }}>{label}</Button>
              ))}
            </Box>

            <Box sx={{ mt: "auto", p: 1.2 }}>
              <Paper variant="outlined" sx={{ p: 1.2, borderRadius: 2.5, bgcolor: "#0d1625" }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}><Avatar sx={{ width: 28, height: 28, bgcolor: "#1b2a4f", color: "primary.light" }}><ShieldCheck size={14} /></Avatar><Box><Typography sx={{ fontSize: 8.5, fontWeight: 800 }}>Safe execution</Typography><Typography color="text.secondary" sx={{ fontSize: 7 }}>Workspace sandbox</Typography></Box></Stack>
                <LinearProgress variant="determinate" value={100} color="success" sx={{ mt: 1, height: 3, borderRadius: 9 }} />
              </Paper>
              <Button fullWidth color="inherit" startIcon={<Settings2 size={13} />} sx={{ mt: .6, justifyContent: "flex-start", color: "text.secondary", fontSize: 8 }}>Workspace settings</Button>
            </Box>
          </Paper>

          <Box component="main" sx={{ minWidth: 0, minHeight: 0, overflowY: "auto", px: { xs: 1.2, sm: 2 }, py: { xs: 1.4, sm: 1.8 }, scrollbarColor: "#34425a transparent" }}>
            <Stack direction={{ xs: "column", sm: "row" }} sx={{ justifyContent: "space-between", alignItems: { xs: "flex-start", sm: "center" }, gap: 1.2, mb: 1.4 }}>
              <Box>
                <Stack direction="row" spacing={.7} sx={{ alignItems: "center" }}><Chip size="small" label="Release workspace" color="primary" variant="outlined" sx={{ height: 22, fontSize: 7 }} /><Typography color="text.secondary" sx={{ fontSize: 8 }}>Updated 38s ago</Typography></Stack>
                <Typography component="h1" sx={{ mt: .7, fontSize: { xs: 19, sm: 23 }, lineHeight: 1.1, fontWeight: 800, letterSpacing: "-.045em" }}>Ship the new agent experience</Typography>
              </Box>
              <Stack direction="row" spacing={.7}>
                <Button variant="outlined" size="small" startIcon={<GitBranch size={13} />} onClick={() => setView(2)}>Review changes</Button>
                <Button variant="contained" size="small" startIcon={<Play size={13} />} onClick={() => setAnnouncement("Preview opened in the project workspace.")}>Open preview</Button>
              </Stack>
            </Stack>

            <Paper variant="outlined" sx={{ mb: 1.4, borderRadius: 3, overflow: "hidden" }}>
              <Tabs value={view} onChange={(_, value: number) => setView(value)} aria-label="Operations views" variant="scrollable" scrollButtons={false} sx={{ minHeight: 42, px: .7, borderBottom: 1, borderColor: "divider", bgcolor: "#0d1625" }}>
                <Tab label="Overview" icon={<LayoutDashboard size={13} />} iconPosition="start" />
                <Tab label="Activity" icon={<Activity size={13} />} iconPosition="start" />
                <Tab label={`Changes · ${changes.length}`} icon={<GitBranch size={13} />} iconPosition="start" />
              </Tabs>
            </Paper>

            {view === 0 && (
              <Stack spacing={1.4}>
                <Paper sx={{ position: "relative", overflow: "hidden", border: "1px solid rgba(124,156,255,.28)", borderRadius: 3, p: { xs: 2, sm: 2.4 }, color: "#f7f9ff", background: "radial-gradient(circle at 86% 12%,rgba(187,156,255,.24),transparent 34%),linear-gradient(120deg,#172652 0%,#243d83 58%,#493584 100%)", boxShadow: "0 20px 52px rgba(0,0,0,.3)" }}>
                  <Box sx={{ position: "absolute", width: 180, height: 180, right: -55, top: -95, border: "32px solid rgba(255,255,255,.08)", borderRadius: "50%" }} />
                  <Stack direction={{ xs: "column", sm: "row" }} sx={{ position: "relative", justifyContent: "space-between", gap: 2 }}>
                    <Box sx={{ maxWidth: 510 }}>
                      <Stack direction="row" spacing={.7} sx={{ alignItems: "center" }}><Sparkles size={14} /><Typography sx={{ fontSize: 8, fontWeight: 800, letterSpacing: ".11em", textTransform: "uppercase", color: "#ced8ff" }}>Current mission</Typography></Stack>
                      <Typography sx={{ mt: 1, fontSize: { xs: 17, sm: 21 }, fontWeight: 780, letterSpacing: "-.035em" }}>Explore ten credible UI directions</Typography>
                      <Typography sx={{ mt: .7, maxWidth: 480, color: "#d9e1ff", fontSize: 9, lineHeight: 1.6 }}>The team is comparing proven agent-workspace patterns, then turning the strongest interactions into responsive prototypes.</Typography>
                    </Box>
                    <Box sx={{ minWidth: 128, alignSelf: "center", textAlign: { xs: "left", sm: "right" } }}><Typography sx={{ fontSize: 27, lineHeight: 1, fontWeight: 800 }}>68%</Typography><Typography sx={{ mt: .5, color: "#cbd6ff", fontSize: 8 }}>mission complete</Typography></Box>
                  </Stack>
                  <LinearProgress variant="determinate" value={68} sx={{ position: "relative", mt: 2, height: 5, borderRadius: 9, bgcolor: "rgba(255,255,255,.18)", "& .MuiLinearProgress-bar": { bgcolor: "#9af1cf", borderRadius: 9 } }} />
                </Paper>

                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(3,minmax(0,1fr))" }, gap: 1 }}>
                  <MiniMetric icon={<UsersRound size={16} />} label="Agent team" value="3 / 4" detail="working" tint="#8da7ff" />
                  <MiniMetric icon={<CheckCircle2 size={16} />} label="Plan" value="2 / 4" detail="complete" tint="#65dcb8" />
                  <MiniMetric icon={<GitBranch size={16} />} label="Review" value="3" detail="files changed" tint="#c0a2ff" />
                </Box>

                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "minmax(0,1.1fr) minmax(230px,.9fr)" }, gap: 1.2 }}>
                  <Paper variant="outlined" sx={{ borderRadius: 3, overflow: "hidden" }}>
                    <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", px: 1.7, py: 1.3, bgcolor: "#0d1625", borderBottom: 1, borderColor: "divider" }}><Box><Typography sx={{ fontSize: 10, fontWeight: 800 }}>Agent workload</Typography><Typography color="text.secondary" sx={{ mt: .2, fontSize: 7.5 }}>Ownership is explicit; blockers surface early.</Typography></Box><Chip size="small" label="Live" color="success" variant="outlined" sx={{ height: 22, fontSize: 7 }} /></Stack>
                    <List disablePadding>
                      {agents.slice(0, 3).map((agent) => {
                        const tone = statusColors[agent.status] ?? { color: "#a4b1c8", bg: "#1a2435" };
                        return (
                          <ListItemButton key={agent.id} selected={selectedAgent === agent.id} onClick={() => setSelectedAgent(agent.id)} sx={{ minHeight: 50, px: 1.5, py: .7, borderBottom: 1, borderColor: "divider", "&:hover": { bgcolor: alpha("#7c9cff", .06) }, "&.Mui-selected": { bgcolor: alpha("#7c9cff", .11) } }}>
                            <ListItemAvatar sx={{ minWidth: 38 }}><Avatar sx={{ width: 29, height: 29, bgcolor: tone.bg, color: tone.color, fontSize: 8, fontWeight: 800 }}>{agent.name.slice(0, 2).toUpperCase()}</Avatar></ListItemAvatar>
                            <ListItemText primary={agent.name} secondary={agent.role} slotProps={{ primary: { sx: { fontSize: 9, fontWeight: 760 } }, secondary: { noWrap: true, sx: { fontSize: 7, mt: .25 } } }} />
                            <Box sx={{ width: 65, mr: 1 }}><LinearProgress variant="determinate" value={agent.progress} sx={{ height: 4, borderRadius: 9, bgcolor: "#202c40", "& .MuiLinearProgress-bar": { bgcolor: tone.color, borderRadius: 9 } }} /><Typography color="text.secondary" sx={{ mt: .35, fontSize: 7, textAlign: "right" }}>{agent.progress}%</Typography></Box>
                            <Chip label={agent.status} size="small" sx={{ height: 21, bgcolor: tone.bg, color: tone.color, fontSize: 7 }} />
                          </ListItemButton>
                        );
                      })}
                    </List>
                  </Paper>

                  <Paper variant="outlined" sx={{ borderRadius: 3, p: 1.6 }}>
                    <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center" }}><Box><Typography sx={{ fontSize: 10, fontWeight: 800 }}>Execution plan</Typography><Typography color="text.secondary" sx={{ mt: .2, fontSize: 7.5 }}>One clear next step.</Typography></Box><IconButton size="small" aria-label="Plan options"><MoreHorizontal size={15} /></IconButton></Stack>
                    <Stack spacing={1.1} sx={{ mt: 1.5 }}>
                      {steps.map((step, index) => (
                        <Stack key={step.label} direction="row" spacing={1} sx={{ alignItems: "center" }}>
                          <Avatar sx={{ width: 23, height: 23, border: 1, borderColor: step.state === "active" ? "primary.main" : "divider", bgcolor: step.state === "done" ? "#15352f" : step.state === "active" ? "#1b2a4f" : "#182233", color: step.state === "done" ? "success.light" : step.state === "active" ? "primary.light" : "text.secondary", fontSize: 8, fontWeight: 800 }}>{step.state === "done" ? <CheckCircle2 size={12} /> : index + 1}</Avatar>
                          <Box sx={{ minWidth: 0, flex: 1 }}><Typography noWrap sx={{ fontSize: 8.5, fontWeight: 700 }}>{step.label}</Typography><Typography color="text.secondary" sx={{ mt: .1, fontSize: 7, textTransform: "capitalize" }}>{step.state}</Typography></Box>
                        </Stack>
                      ))}
                    </Stack>
                  </Paper>
                </Box>
              </Stack>
            )}

            {view === 1 && (
              <Paper variant="outlined" sx={{ borderRadius: 3, p: { xs: 1.5, sm: 2 } }}>
                <Typography sx={{ fontSize: 14, fontWeight: 800 }}>Live activity</Typography>
                <Typography color="text.secondary" sx={{ mt: .3, mb: 2, fontSize: 8.5 }}>A shared timeline makes parallel work legible without opening every transcript.</Typography>
                {extendedActivity.map((item, index) => (
                  <Stack key={`${item.time}-${item.agent}`} direction="row" spacing={1.3} sx={{ minHeight: 55 }}>
                    <Typography color="text.secondary" sx={{ width: 31, pt: .25, fontSize: 7.5, fontFamily: "ui-monospace, monospace" }}>{item.time}</Typography>
                    <Stack sx={{ alignItems: "center" }}><Avatar sx={{ width: 22, height: 22, bgcolor: item.tone === "done" ? "#15352f" : item.tone === "running" ? "#1b2a4f" : "#2b2141", color: item.tone === "done" ? "success.light" : item.tone === "running" ? "primary.light" : "secondary.light" }}>{item.tone === "done" ? <CheckCircle2 size={12} /> : item.tone === "running" ? <Activity size={12} /> : <GitBranch size={12} />}</Avatar>{index < activity.length + 1 && <Box sx={{ width: 1, flex: 1, bgcolor: "divider" }} />}</Stack>
                    <Box sx={{ pb: 1.5 }}><Typography sx={{ fontSize: 8.5, fontWeight: 800 }}>{item.agent}</Typography><Typography color="text.secondary" sx={{ mt: .35, fontSize: 8.5 }}>{item.text}</Typography></Box>
                  </Stack>
                ))}
              </Paper>
            )}

            {view === 2 && (
              <Paper variant="outlined" sx={{ borderRadius: 3, overflow: "hidden" }}>
                <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", p: 1.7, bgcolor: "#0d1625", borderBottom: 1, borderColor: "divider" }}><Box><Typography sx={{ fontSize: 13, fontWeight: 800 }}>Ready for review</Typography><Typography color="text.secondary" sx={{ mt: .25, fontSize: 8 }}>Three files stay grouped by the task that produced them.</Typography></Box><Button variant="contained" size="small" startIcon={<CheckCircle2 size={13} />}>Approve set</Button></Stack>
                {changes.map((change) => (
                  <Button key={change.file} fullWidth color="inherit" onClick={() => setAnnouncement(`${change.file} opened in review.`)} sx={{ display: "grid", gridTemplateColumns: "34px minmax(0,1fr) auto 18px", gap: 1, minHeight: 58, px: 1.7, borderBottom: 1, borderColor: "divider", borderRadius: 0, textAlign: "left" }}>
                    <Avatar variant="rounded" sx={{ width: 31, height: 31, bgcolor: "#1b2a4f", color: "primary.light" }}><FileCode2 size={15} /></Avatar>
                    <Box sx={{ minWidth: 0 }}><Typography noWrap sx={{ fontFamily: "ui-monospace, monospace", fontSize: 8.5, fontWeight: 700 }}>{change.file}</Typography><Typography color="text.secondary" sx={{ mt: .3, fontSize: 7.5 }}>Modified by ui-builder</Typography></Box>
                    <Chip label={change.delta} color="success" variant="outlined" size="small" sx={{ height: 22, fontSize: 7 }} /><ChevronRight size={14} />
                  </Button>
                ))}
              </Paper>
            )}
          </Box>

          <Paper component="aside" square elevation={0} aria-label="Agent command panel" sx={{ display: { xs: "none", lg: "flex" }, minHeight: 0, flexDirection: "column", overflow: "hidden", borderLeft: 1, borderColor: "divider" }}>
            <Box sx={{ p: 1.6, borderBottom: 1, borderColor: "divider", bgcolor: "#0d1625" }}>
              <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center" }}><Box><Typography sx={{ fontSize: 10, fontWeight: 800 }}>Agent team</Typography><Typography color="text.secondary" sx={{ mt: .25, fontSize: 7.5 }}>Direct ownership, visible state.</Typography></Box><MuiBadge badgeContent={3} color="primary"><UsersRound size={16} /></MuiBadge></Stack>
            </Box>
            <List disablePadding>
              {agents.map((agent) => {
                const tone = statusColors[agent.status] ?? { color: "#a4b1c8", bg: "#1a2435" };
                return (
                  <ListItemButton key={agent.id} selected={selectedAgent === agent.id} onClick={() => setSelectedAgent(agent.id)} sx={{ minHeight: 52, px: 1.4, borderBottom: 1, borderColor: "divider", "&:hover": { bgcolor: alpha("#7c9cff", .06) }, "&.Mui-selected": { bgcolor: alpha("#7c9cff", .12), boxShadow: "inset 3px 0 #7c9cff" } }}>
                    <ListItemAvatar sx={{ minWidth: 37 }}><Avatar sx={{ width: 29, height: 29, bgcolor: tone.bg, color: tone.color, fontSize: 8, fontWeight: 800 }}>{agent.name.slice(0, 2).toUpperCase()}</Avatar></ListItemAvatar>
                    <ListItemText primary={agent.name} secondary={`${agent.progress}% · ${agent.role}`} slotProps={{ primary: { noWrap: true, sx: { fontSize: 9, fontWeight: 760 } }, secondary: { noWrap: true, sx: { fontSize: 7, mt: .3 } } }} />
                    <Box sx={{ width: 6, height: 6, borderRadius: 9, bgcolor: tone.color }} />
                  </ListItemButton>
                );
              })}
            </List>

            <Box sx={{ p: 1.4 }}>
              <Paper variant="outlined" sx={{ p: 1.4, borderRadius: 2.5, background: "linear-gradient(145deg,#14213a,#101827)" }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}><Avatar sx={{ width: 32, height: 32, bgcolor: "#1b2a4f", color: "primary.light" }}><Bot size={15} /></Avatar><Box sx={{ minWidth: 0 }}><Typography noWrap sx={{ fontSize: 9.5, fontWeight: 800 }}>{activeAgent.name}</Typography><Typography noWrap color="text.secondary" sx={{ mt: .2, fontSize: 7.5 }}>{activeAgent.role}</Typography></Box></Stack>
                <Stack direction="row" sx={{ justifyContent: "space-between", mt: 1.3 }}><Typography color="text.secondary" sx={{ fontSize: 7.5 }}>Current progress</Typography><Typography sx={{ fontSize: 7.5, fontWeight: 800 }}>{activeAgent.progress}%</Typography></Stack>
                <LinearProgress variant="determinate" value={activeAgent.progress} sx={{ mt: .6, height: 4, borderRadius: 9 }} />
              </Paper>
            </Box>

            <Box component="form" onSubmit={submitCommand} sx={{ mt: "auto", p: 1.4, borderTop: 1, borderColor: "divider", bgcolor: "#0d1625" }}>
              <Typography sx={{ mb: .8, color: "text.secondary", fontSize: 7.5, fontWeight: 800, letterSpacing: ".08em", textTransform: "uppercase" }}>Guide selected agent</Typography>
              <TextField
                fullWidth
                multiline
                minRows={2}
                maxRows={4}
                value={command}
                onChange={(event) => setCommand(event.target.value)}
                placeholder={`Message ${activeAgent.name}…`}
                slotProps={{
                  htmlInput: { "aria-label": `Message ${activeAgent.name}` },
                  input: { startAdornment: <InputAdornment position="start" sx={{ alignSelf: "flex-start", mt: .7 }}><Command size={14} /></InputAdornment> },
                }}
                sx={{ "& .MuiInputBase-root": { bgcolor: "#0a111e", fontSize: 9 }, "& textarea::placeholder": { opacity: .72 } }}
              />
              <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", mt: .8 }}><Typography color="text.secondary" sx={{ fontSize: 7 }}>Enter to queue guidance</Typography><Button type="submit" variant="contained" size="small" disabled={!command.trim()} endIcon={<Send size={12} />}>Send</Button></Stack>
            </Box>
          </Paper>
        </Box>
        <Box component="span" role="status" sx={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" }}>{announcement}</Box>
      </Box>
    </ThemeProvider>
  );
}

export default MUIOperationsCockpit;
