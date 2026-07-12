import { useCallback, useEffect, useRef, useState } from "react";
import {
  emptyConversation,
  hydrateMessages,
  reduceEvent,
  type ChatMessage,
  type ConversationState,
} from "../lib/conversation";
import { unsupported, type DaemonClient } from "../lib/daemon-client";
import { writeAppStorage } from "../lib/storage";
import {
  addUsage,
  attachmentHTTPURL,
  conversationKind,
  conversationScope,
  normalizeRuntimeState,
  normalizeUsage,
  wireConversationID,
  type CapabilityState,
  type Command,
  type ConnectionStatus,
  type Conversation,
  type RuntimeState,
  type ImageAttachment,
  type WireEvent,
} from "../lib/protocol";

const emptyCapabilities: CapabilityState = { capabilities: [], skills: [], skill_diagnostics: [] };

interface SessionOptions {
  client: DaemonClient | null;
  connection: ConnectionStatus;
  connectionVersion: number;
  selected: Conversation | null;
  onUpsert: (conversation: Conversation) => void;
  onActivity: (id: string) => void;
  onNotice: (message: string) => void;
}

export interface ConversationSession {
  runtime: RuntimeState | null;
  conversation: ConversationState;
  capabilities: CapabilityState;
  historyLoading: boolean;
  handleEvent: (event: WireEvent) => void;
  sync: () => Promise<void>;
  appendLocal: (message: Omit<ChatMessage, "id">) => void;
  clearTranscript: () => void;
  update: (command: Omit<Command, "id">, notice?: string) => Promise<Conversation | null>;
  send: (kind: "prompt" | "steer" | "follow-up", value: string, attachments?: ImageAttachment[]) => Promise<void>;
  abort: () => Promise<void>;
}

interface SessionSnapshot {
  runtime: RuntimeState | null;
  conversation: ConversationState;
  capabilities: CapabilityState;
}

const maxCachedSessions = 16;

function rememberSession(cache: Map<string, SessionSnapshot>, key: string, snapshot: SessionSnapshot) {
  cache.delete(key);
  cache.set(key, snapshot);
  while (cache.size > maxCachedSessions) {
    const oldest = cache.keys().next().value;
    if (typeof oldest !== "string") break;
    cache.delete(oldest);
  }
}

export function useConversationSession(options: SessionOptions): ConversationSession {
  const { client, connection, connectionVersion, selected, onActivity, onNotice, onUpsert } = options;
  const [runtime, setRuntime] = useState<RuntimeState | null>(null);
  const [conversation, setConversation] = useState<ConversationState>(emptyConversation);
  const [capabilities, setCapabilities] = useState<CapabilityState>(emptyCapabilities);
  const [sessionKey, setSessionKey] = useState("");
  const [historyLoading, setHistoryLoading] = useState(false);
  const selectedRef = useRef(selected);
  const clientRef = useRef(client);
  const runtimeRef = useRef(runtime);
  const conversationRef = useRef(conversation);
  const capabilitiesRef = useRef(capabilities);
  const historyLoadingRef = useRef(historyLoading);
  const cacheNamespace = useRef("");
  const sessionCache = useRef(new Map<string, SessionSnapshot>());
  const lastSequence = useRef<Record<string, number>>({});
  const syncGenerations = useRef<Record<string, number>>({});
  const activeSelection = useRef("");
  const syncRef = useRef<() => void>(() => undefined);
  if (client?.url) cacheNamespace.current = client.url;
  selectedRef.current = selected;
  clientRef.current = client;
  runtimeRef.current = runtime;
  conversationRef.current = conversation;
  capabilitiesRef.current = capabilities;
  historyLoadingRef.current = historyLoading;

  const keyFor = useCallback((id: string, source: DaemonClient | null = client) =>
    `${source?.url || cacheNamespace.current}\u0000${id}`,
  [client]);

  const sync = useCallback(async () => {
    const resource = selectedRef.current;
    const activeClient = client;
    if (!activeClient?.isOpen || !resource) return;
    const id = resource.id;
    const key = keyFor(id, activeClient);
    const generation = (syncGenerations.current[key] || 0) + 1;
    syncGenerations.current[key] = generation;
    try {
      await activeClient.subscribe(resource);
      const after = lastSequence.current[key] || 0;
      const [canonical, state, messages, events, nextCapabilities] = await Promise.all([
        activeClient.getConversation(resource),
        activeClient.getState(resource),
        activeClient.getMessages(resource),
        activeClient.getEvents(resource, after),
        activeClient.getCapabilities(resource).catch((error) => {
          if (!unsupported(error)) throw error;
          return emptyCapabilities;
        }),
      ]);
      if (syncGenerations.current[key] !== generation) return;
      const maxSequence = events.reduce(
        (maximum, event) => Math.max(maximum, event.sequence),
        after,
      );
      lastSequence.current[key] = maxSequence;
      const normalized = normalizeRuntimeState(state, canonical);
      const cached = sessionCache.current.get(key);
      const currentConversation = activeSelection.current === key
        ? conversationRef.current
        : cached?.conversation || { ...emptyConversation, lastSequence: after };
      const nextConversation: ConversationState = {
        ...currentConversation,
        messages: hydrateMessages(messages, (file) => attachmentHTTPURL(activeClient.url, resource.id, file)),
        stream: normalized.running ? currentConversation.stream : null,
        reasoning: normalized.running ? currentConversation.reasoning : null,
        activeTools: normalized.running ? currentConversation.activeTools : [],
        running: normalized.running,
        steeringQueued: normalized.steering_queued,
        followUpsQueued: normalized.follow_ups_queued,
        lastSequence: maxSequence,
        hasSequenceGap: false,
      };
      const snapshot = { runtime: normalized, conversation: nextConversation, capabilities: nextCapabilities };
      rememberSession(sessionCache.current, key, snapshot);
      if (clientRef.current === activeClient) onUpsert(normalized.conversation);
      if (activeSelection.current !== key || selectedRef.current?.id !== id || clientRef.current !== activeClient) return;
      runtimeRef.current = normalized;
      conversationRef.current = nextConversation;
      capabilitiesRef.current = nextCapabilities;
      historyLoadingRef.current = false;
      setRuntime(normalized);
      setConversation(nextConversation);
      setCapabilities(nextCapabilities);
      setHistoryLoading(false);
    } catch (error) {
      if (syncGenerations.current[key] !== generation) return;
      if (activeSelection.current === key) {
        historyLoadingRef.current = false;
        setHistoryLoading(false);
      }
      onNotice(error instanceof Error ? error.message : "Could not load conversation");
    }
  }, [client, keyFor, onNotice, onUpsert]);

  syncRef.current = () => void sync();

  useEffect(() => {
    if (!selected) {
      if (activeSelection.current && !historyLoadingRef.current) {
        rememberSession(sessionCache.current, activeSelection.current, {
          runtime: runtimeRef.current,
          conversation: conversationRef.current,
          capabilities: capabilitiesRef.current,
        });
      }
      activeSelection.current = "";
      setSessionKey("");
      setHistoryLoading(false);
      setRuntime(null);
      setConversation(emptyConversation);
      setCapabilities(emptyCapabilities);
      return;
    }
    writeAppStorage("conversation", selected.id);
    const key = keyFor(selected.id);
    if (activeSelection.current !== key) {
      if (activeSelection.current && !historyLoadingRef.current) {
        rememberSession(sessionCache.current, activeSelection.current, {
          runtime: runtimeRef.current,
          conversation: conversationRef.current,
          capabilities: capabilitiesRef.current,
        });
      }
      activeSelection.current = key;
      setSessionKey(key);
      const cached = sessionCache.current.get(key);
      if (cached) {
        sessionCache.current.delete(key);
        sessionCache.current.set(key, cached);
        runtimeRef.current = cached.runtime;
        conversationRef.current = cached.conversation;
        capabilitiesRef.current = cached.capabilities;
        historyLoadingRef.current = false;
        setRuntime(cached.runtime);
        setConversation(cached.conversation);
        setCapabilities(cached.capabilities);
        setHistoryLoading(false);
      } else {
        const nextConversation = {
          ...emptyConversation,
          lastSequence: lastSequence.current[key] || 0,
        };
        runtimeRef.current = null;
        conversationRef.current = nextConversation;
        capabilitiesRef.current = emptyCapabilities;
        historyLoadingRef.current = true;
        setRuntime(null);
        setConversation(nextConversation);
        setCapabilities(emptyCapabilities);
        setHistoryLoading(true);
      }
    }
    // A disconnect disables mutations but should not make a persisted
    // conversation look empty. Re-sync the same view when the client returns.
    if (connection !== "online" || !client) return;
    void sync();
    return () => {
      if (client.isOpen) void client.unsubscribe(selected).catch(() => undefined);
    };
  }, [client, connection, connectionVersion, keyFor, selected?.id, sync]);

  useEffect(() => {
    if (!sessionKey || historyLoading) return;
    rememberSession(sessionCache.current, sessionKey, { runtime, conversation, capabilities });
  }, [capabilities, conversation, historyLoading, runtime, sessionKey]);

  const handleEvent = useCallback((event: WireEvent) => {
    const id = wireConversationID(event);
    if (!id) return;
    const key = keyFor(id);
    if (event.sequence) {
      const previous = lastSequence.current[key] || 0;
      if (previous === 0 || event.sequence === previous + 1) {
        lastSequence.current[key] = event.sequence;
      }
    }
    const active = selectedRef.current;
    if (!active || active.id !== id) return;
    if (event.type === "agent_start" || event.type === "agent_settled") {
      onUpsert({
        ...active,
        status: event.type === "agent_start" ? "running" : "idle",
      });
    }
    const isUpdate = ["conversation_updated", "chat_updated", "project_updated", "thread_updated"]
      .includes(event.type);
    if (isUpdate && event.data && typeof event.data.id === "string") {
      onUpsert(event.data as unknown as Conversation);
    }
    setConversation((current) => {
      const next = reduceEvent(current, event);
      if (next.hasSequenceGap && !current.hasSequenceGap) queueMicrotask(syncRef.current);
      return next;
    });
    setRuntime((current) => {
      if (!current) return current;
      if (event.type === "agent_start") return { ...current, running: true };
      if (event.type === "agent_settled") return { ...current, running: false };
      if (event.type === "message_end" && event.data?.usage) {
        return { ...current, usage: addUsage(current.usage, event.data.usage) };
      }
      if (event.type === "usage_update") {
        return { ...current, usage: normalizeUsage(event.data) };
      }
      if (event.type === "queue_update") {
        return {
          ...current,
          steering_queued: Number(event.data?.steering || 0),
          follow_ups_queued: Number(event.data?.follow_ups || 0),
        };
      }
      if (isUpdate && event.data && typeof event.data.id === "string") {
        return normalizeRuntimeState(
          { ...current, conversation: event.data as unknown as Conversation },
          event.data as unknown as Conversation,
        );
      }
      return current;
    });
    if (event.type === "agent_settled" || event.type === "capabilities_updated") {
      window.setTimeout(syncRef.current, 80);
    }
  }, [keyFor, onUpsert]);

  const appendLocal = useCallback((message: Omit<ChatMessage, "id">) => {
    setConversation((current) => ({
      ...current,
      messages: [...current.messages, {
        ...message,
        id: `local-${Date.now()}-${Math.random()}`,
      }],
    }));
  }, []);

  const clearTranscript = useCallback(() => {
    setConversation((current) => ({ ...current, messages: [], stream: null, reasoning: null }));
  }, []);

  const update = useCallback(async (
    command: Omit<Command, "id">,
    notice?: string,
  ): Promise<Conversation | null> => {
    const resource = selectedRef.current;
    if (!client?.isOpen || !resource) return null;
    try {
      const updated = await client.request<Conversation>({
        ...command,
        ...conversationScope(resource),
      });
      const canonical = { ...updated, kind: conversationKind(resource) };
      setRuntime((current) => current
        ? normalizeRuntimeState({ ...current, conversation: canonical }, canonical)
        : current);
      onUpsert(canonical);
      if (command.type === "set_mcp_server_enabled") {
        void client.getCapabilities(canonical).then((nextCapabilities) => {
          if (selectedRef.current?.id !== resource.id) return;
          capabilitiesRef.current = nextCapabilities;
          setCapabilities(nextCapabilities);
        }).catch(() => undefined);
      }
      if (notice) onNotice(notice);
      return canonical;
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Setting update failed");
      return null;
    }
  }, [client, onNotice, onUpsert]);

  const send = useCallback(async (
    kind: "prompt" | "steer" | "follow-up",
    value: string,
    attachments: ImageAttachment[] = [],
  ) => {
    const resource = selectedRef.current;
    if (!client?.isOpen || !resource) return;
    const wasRunning = Boolean(runtimeRef.current?.running);
    appendLocal({
      role: "user",
      content: value,
      label: kind === "prompt" ? undefined : kind,
      pending: true,
      attachments: attachments.map((attachment) => ({
        name: attachment.name || "Pasted image",
        mimeType: attachment.mime_type,
        url: attachment.data ? `data:${attachment.mime_type};base64,${attachment.data}` : "",
      })).filter((attachment) => Boolean(attachment.url)),
    });
    setConversation((current) => ({ ...current, running: true }));
    setRuntime((current) => current ? { ...current, running: true } : current);
    try {
      await client.request<void>({
        type: kind === "follow-up" ? "follow_up" : kind,
        ...conversationScope(resource),
        message: value,
        attachments,
      });
      onActivity(resource.id);
    } catch (error) {
      setConversation((current) => ({ ...current, running: wasRunning }));
      setRuntime((current) => current ? { ...current, running: wasRunning } : current);
      appendLocal({ role: "error", content: error instanceof Error ? error.message : "Message failed" });
    }
  }, [appendLocal, client, onActivity]);

  const abort = useCallback(async () => {
    const resource = selectedRef.current;
    if (!client?.isOpen || !resource) return;
    try {
      await client.request<void>({ type: "abort", ...conversationScope(resource) });
      onNotice("Abort requested");
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "Abort failed");
    }
  }, [client, onNotice]);

  const selectedKey = selected ? keyFor(selected.id) : "";
  const cached = selectedKey && sessionKey !== selectedKey ? sessionCache.current.get(selectedKey) : undefined;
  const visibleRuntime = cached ? cached.runtime : sessionKey === selectedKey ? runtime : null;
  const visibleConversation = cached
    ? cached.conversation
    : sessionKey === selectedKey
      ? conversation
      : { ...emptyConversation, lastSequence: lastSequence.current[selectedKey] || 0 };
  const visibleCapabilities = cached
    ? cached.capabilities
    : sessionKey === selectedKey ? capabilities : emptyCapabilities;
  const visibleHistoryLoading = Boolean(selected) && !cached && (sessionKey !== selectedKey || historyLoading);

  return {
    runtime: visibleRuntime,
    conversation: visibleConversation,
    capabilities: visibleCapabilities,
    historyLoading: visibleHistoryLoading,
    handleEvent,
    sync,
    appendLocal,
    clearTranscript,
    update,
    send,
    abort,
  };
}
