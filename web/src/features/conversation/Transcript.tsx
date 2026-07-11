import { Bot, Brain, Sparkles, Wrench } from "lucide-react";
import { useLayoutEffect, useRef } from "react";
import type { UIEvent } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { ChatMessage, ConversationState } from "../../lib/conversation";
import { summarizeArguments } from "../../lib/display";

interface TranscriptProps {
  conversationID: string;
  state: ConversationState;
  loading: boolean;
  onPrompt: (prompt: string) => void;
}

interface ScrollPosition {
  top: number;
  atBottom: boolean;
}

const maxRememberedConversations = 32;
const bottomThreshold = 80;

export function Transcript({ conversationID, state, loading, onPrompt }: TranscriptProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const positions = useRef(new Map<string, ScrollPosition>());
  const positionedConversation = useRef<string | null>(null);
  const pinnedToBottom = useRef(true);

  useLayoutEffect(() => {
    const scroll = scrollRef.current;
    if (!scroll || loading) return;

    const switchedConversation = positionedConversation.current !== conversationID;
    if (switchedConversation) {
      const saved = positions.current.get(conversationID);
      const bottom = maxScrollTop(scroll);
      scroll.scrollTop = saved
        ? saved.atBottom ? bottom : Math.min(saved.top, bottom)
        : bottom;
      pinnedToBottom.current = saved?.atBottom ?? true;
      positionedConversation.current = conversationID;
      rememberPosition(positions.current, conversationID, scroll);
      return;
    }

    if (pinnedToBottom.current) {
      scroll.scrollTop = maxScrollTop(scroll);
      rememberPosition(positions.current, conversationID, scroll);
    }
  }, [
    conversationID,
    loading,
    state.messages,
    state.stream?.id,
    state.stream?.content,
    state.reasoning?.id,
    state.reasoning?.content,
    state.activeTools,
  ]);

  useLayoutEffect(() => {
    const scroll = scrollRef.current;
    return () => {
      if (scroll && !loading) {
        rememberPosition(positions.current, conversationID, scroll);
      }
    };
  }, [conversationID, loading]);

  function handleScroll(event: UIEvent<HTMLDivElement>) {
    if (loading) return;
    pinnedToBottom.current = isNearBottom(event.currentTarget);
    rememberPosition(positions.current, conversationID, event.currentTarget);
  }

  return (
    <div
      ref={scrollRef}
      className="message-scroll h-0 min-h-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-contain"
      data-testid="message-scroll"
      onScroll={handleScroll}
    >
      {loading ? (
        <div
          className="flex min-h-full items-center justify-center gap-2 px-6 text-sm text-slate-400"
          role="status"
          aria-live="polite"
        >
          <span className="tool-loader" aria-hidden="true" />
          <span>Loading conversation…</span>
        </div>
      ) : (
        <div className="message-column mx-auto w-full max-w-[820px] px-[clamp(20px,5vw,44px)] pt-7 pb-11">
          {state.messages.length === 0 && !state.stream && <WelcomeState onPrompt={onPrompt} />}
          {state.messages.map((message) => <MessageCard key={message.id} message={message} />)}
          {state.reasoning && (
            <MessageCard
              message={{ id: state.reasoning.id, role: "reasoning", content: state.reasoning.content }}
              streaming
            />
          )}
          {state.activeTools.map((tool) => (
            <div className="tool-live" key={tool.id}>
              <div className="tool-live-icon"><Wrench size={15} /></div>
              <div>
                <strong>Running {tool.name}</strong>
                <span>{summarizeArguments(tool.arguments)}</span>
              </div>
              <span className="tool-loader" />
            </div>
          ))}
          {state.stream?.content && (
            <MessageCard
              message={{
                id: state.stream.id,
                role: "assistant",
                content: state.stream.content,
              }}
              streaming
            />
          )}
        </div>
      )}
    </div>
  );
}

function maxScrollTop(element: HTMLElement): number {
  return Math.max(0, element.scrollHeight - element.clientHeight);
}

function isNearBottom(element: HTMLElement): boolean {
  return maxScrollTop(element) - element.scrollTop <= bottomThreshold;
}

function rememberPosition(
  positions: Map<string, ScrollPosition>,
  conversationID: string,
  element: HTMLElement,
) {
  if (!conversationID) return;
  positions.delete(conversationID);
  positions.set(conversationID, {
    top: element.scrollTop,
    atBottom: isNearBottom(element),
  });
  while (positions.size > maxRememberedConversations) {
    const oldest = positions.keys().next().value;
    if (oldest === undefined) break;
    positions.delete(oldest);
  }
}

function MessageCard({ message, streaming = false }: { message: ChatMessage; streaming?: boolean }) {
  if (message.role === "user") {
    return (
      <article className="message user-message">
        <div className="message-avatar user-avatar">Y</div>
        <div className="message-body">
          <div className="message-meta">
            <strong>You</strong>
            {message.label && <span className="message-label">{message.label}</span>}
            {message.pending && <span className="pending-label">sent</span>}
          </div>
          <p>{message.content}</p>
          {message.attachments && message.attachments.length > 0 && (
            <div className="message-images">
              {message.attachments.map((image, index) => (
                <a href={image.url} target="_blank" rel="noreferrer" key={`${image.url}-${index}`}>
                  <img src={image.url} alt={image.name} />
                </a>
              ))}
            </div>
          )}
        </div>
      </article>
    );
  }
  if (message.role === "tool" || message.role === "error") {
    return (
      <article className={`tool-result ${message.role === "error" ? "failed" : ""}`}>
        <div className="tool-result-heading">
          <Wrench size={13} />
          <strong>{message.label || (message.role === "error" ? "Agent error" : "Tool result")}</strong>
        </div>
        {message.arguments !== undefined && (
          <div className="tool-result-section">
            <small>Input</small>
            <pre>{formatToolValue(message.arguments)}</pre>
          </div>
        )}
        <div className="tool-result-section">
          <small>{message.role === "error" ? "Error" : "Output"}</small>
          <pre>{message.content}</pre>
        </div>
      </article>
    );
  }
  if (message.role === "reasoning") {
    return (
      <details className="reasoning-card" open>
        <summary><Brain size={13} /><strong>Thinking</strong>{streaming && <span className="streaming-label">streaming</span>}</summary>
        <div className="markdown-body">
          <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{message.content || "Thinking…"}</ReactMarkdown>
          {streaming && <span className="stream-caret" />}
        </div>
      </details>
    );
  }
  return (
    <article className="message assistant-message">
      <div className="message-avatar assistant-avatar"><Bot size={13} /></div>
      <div className="message-body">
        <div className="message-meta">
          <strong>{message.role === "system" ? "Dire Agent" : "Agent"}</strong>
          {streaming && <span className="streaming-label">streaming</span>}
        </div>
        <div className="markdown-body">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
          {streaming && <span className="stream-caret" />}
        </div>
      </div>
    </article>
  );
}

function formatToolValue(value: unknown): string {
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function WelcomeState({ onPrompt }: { onPrompt: (value: string) => void }) {
  const prompts = [
    "Inspect this codebase and explain its architecture",
    "Find the highest-impact improvement to make next",
    "Review the current changes and run the relevant tests",
  ];
  return (
    <div className="welcome-state">
      <div className="welcome-icon"><Sparkles size={21} /></div>
      <span className="eyebrow">READY WHEN YOU ARE</span>
      <h2>What should the agent work on?</h2>
      <p>The conversation, events, model state, and token usage persist in their own SQLite file.</p>
      <div className="prompt-grid">
        {prompts.map((prompt) => (
          <button key={prompt} onClick={() => onPrompt(prompt)}>
            <Sparkles size={14} /> {prompt}
          </button>
        ))}
      </div>
    </div>
  );
}
