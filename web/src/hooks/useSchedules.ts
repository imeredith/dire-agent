import { useCallback, useEffect, useRef, useState } from "react";
import { unsupported, type DaemonClient } from "../lib/daemon-client";
import type {
  ScheduledPrompt,
  ScheduledPromptInput,
  WireEvent,
} from "../lib/protocol";

interface ScheduleOptions {
  client: DaemonClient | null;
  active: boolean;
  connectionVersion: number;
  onNotice: (message: string) => void;
}

export interface SchedulesController {
  schedules: ScheduledPrompt[];
  loading: boolean;
  supported: boolean;
  error: string;
  busy: string;
  reload: () => Promise<void>;
  create: (input: ScheduledPromptInput) => Promise<ScheduledPrompt | null>;
  update: (id: string, input: ScheduledPromptInput) => Promise<ScheduledPrompt | null>;
  remove: (id: string) => Promise<boolean>;
  runNow: (id: string) => Promise<boolean>;
  handleEvent: (event: WireEvent) => void;
}

function sortSchedules(values: ScheduledPrompt[]): ScheduledPrompt[] {
  return [...values].sort((left, right) => {
    const leftNext = left.next_run_at || "9999";
    const rightNext = right.next_run_at || "9999";
    return leftNext.localeCompare(rightNext) || left.name.localeCompare(right.name);
  });
}

function scheduledPromptFromEvent(event: WireEvent): ScheduledPrompt | null {
  const data = event.data;
  if (!data) return null;
  const candidate = data.schedule && typeof data.schedule === "object"
    ? data.schedule as Record<string, unknown>
    : data;
  return typeof candidate.id === "string" && typeof candidate.name === "string"
    ? candidate as unknown as ScheduledPrompt
    : null;
}

export function scheduledPromptInput(schedule: ScheduledPrompt): ScheduledPromptInput {
  return {
    name: schedule.name,
    prompt: schedule.prompt,
    target_type: schedule.target_type,
    ...(schedule.conversation_id ? { conversation_id: schedule.conversation_id } : {}),
    schedule_type: schedule.schedule_type,
    ...(schedule.cron ? { cron: schedule.cron } : {}),
    timezone: schedule.timezone,
    ...(schedule.run_at ? { run_at: schedule.run_at } : {}),
    enabled: schedule.enabled,
  };
}

export function useSchedules(options: ScheduleOptions): SchedulesController {
  const { client, active, connectionVersion, onNotice } = options;
  const [schedules, setSchedules] = useState<ScheduledPrompt[]>([]);
  const [loading, setLoading] = useState(false);
  const [supported, setSupported] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const activeRef = useRef(active);
  const reloadRef = useRef<() => void>(() => undefined);
  activeRef.current = active;

  const reload = useCallback(async () => {
    if (!client?.isOpen) return;
    setLoading(true);
    setError("");
    try {
      const next = await client.listScheduledPrompts();
      setSchedules(sortSchedules(next));
      setSupported(true);
    } catch (cause) {
      if (unsupported(cause)) {
        setSupported(false);
        setSchedules([]);
      } else {
        setError(cause instanceof Error ? cause.message : "Could not load scheduled prompts");
      }
    } finally {
      setLoading(false);
    }
  }, [client]);
  reloadRef.current = () => void reload();

  useEffect(() => {
    if (!active || !client?.isOpen) return;
    let cancelled = false;
    void (async () => {
      try {
        await client.subscribeScheduledPrompts();
      } catch (cause) {
        if (unsupported(cause)) setSupported(false);
        return;
      }
      if (cancelled) {
        if (client.isOpen) await client.unsubscribeScheduledPrompts().catch(() => undefined);
        return;
      }
      await reload();
    })();
    return () => {
      cancelled = true;
      if (client.isOpen) void client.unsubscribeScheduledPrompts().catch(() => undefined);
    };
  }, [active, client, connectionVersion, reload]);

  const upsert = useCallback((schedule: ScheduledPrompt) => {
    setSchedules((current) => {
      const existing = current.find((item) => item.id === schedule.id);
      if (existing && Date.parse(existing.updated_at) > Date.parse(schedule.updated_at)) return current;
      return sortSchedules([
        schedule,
        ...current.filter((item) => item.id !== schedule.id),
      ]);
    });
  }, []);

  const create = useCallback(async (input: ScheduledPromptInput): Promise<ScheduledPrompt | null> => {
    if (!client?.isOpen) return null;
    setBusy("create");
    setError("");
    try {
      const created = await client.createScheduledPrompt(input);
      upsert(created);
      setSupported(true);
      onNotice("Scheduled prompt created");
      return created;
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not create scheduled prompt";
      setError(message);
      onNotice(message);
      return null;
    } finally {
      setBusy("");
    }
  }, [client, onNotice, upsert]);

  const update = useCallback(async (id: string, input: ScheduledPromptInput): Promise<ScheduledPrompt | null> => {
    if (!client?.isOpen) return null;
    setBusy(`update:${id}`);
    setError("");
    try {
      const updated = await client.updateScheduledPrompt(id, input);
      upsert(updated);
      onNotice("Scheduled prompt updated");
      return updated;
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not update scheduled prompt";
      setError(message);
      onNotice(message);
      return null;
    } finally {
      setBusy("");
    }
  }, [client, onNotice, upsert]);

  const remove = useCallback(async (id: string): Promise<boolean> => {
    if (!client?.isOpen) return false;
    setBusy(`delete:${id}`);
    setError("");
    try {
      await client.deleteScheduledPrompt(id);
      setSchedules((current) => current.filter((item) => item.id !== id));
      onNotice("Scheduled prompt deleted");
      return true;
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not delete scheduled prompt";
      setError(message);
      onNotice(message);
      return false;
    } finally {
      setBusy("");
    }
  }, [client, onNotice]);

  const runNow = useCallback(async (id: string): Promise<boolean> => {
    if (!client?.isOpen) return false;
    setBusy(`run:${id}`);
    setError("");
    try {
      const updated = await client.runScheduledPrompt(id);
      if (updated?.id) upsert(updated);
      else window.setTimeout(reloadRef.current, 80);
      onNotice("Scheduled prompt started");
      return true;
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "Could not run scheduled prompt";
      setError(message);
      onNotice(message);
      return false;
    } finally {
      setBusy("");
    }
  }, [client, onNotice, upsert]);

  const handleEvent = useCallback((event: WireEvent) => {
    if (!event.type.includes("scheduled_prompt")) return;
    const schedule = scheduledPromptFromEvent(event);
    if (event.type.includes("deleted")) {
      const id = schedule?.id || (typeof event.data?.schedule_id === "string" ? event.data.schedule_id : "");
      if (id) setSchedules((current) => current.filter((item) => item.id !== id));
      return;
    }
    if (schedule) {
      upsert(schedule);
      return;
    }
    if (activeRef.current) window.setTimeout(reloadRef.current, 80);
  }, [upsert]);

  return {
    schedules,
    loading,
    supported,
    error,
    busy,
    reload,
    create,
    update,
    remove,
    runNow,
    handleEvent,
  };
}
