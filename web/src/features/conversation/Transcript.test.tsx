import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { emptyConversation, type ConversationState } from "../../lib/conversation";
import { Transcript } from "./Transcript";

let contentHeight = 1_000;
const viewportHeight = 200;

function conversation(message: string): ConversationState {
  return {
    ...emptyConversation,
    messages: [{ id: message, role: "assistant", content: message }],
  };
}

describe("Transcript scrolling", () => {
  beforeEach(() => {
    contentHeight = 1_000;
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get")
      .mockImplementation(() => contentHeight);
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get")
      .mockImplementation(() => viewportHeight);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows only a centered status while an uncached conversation loads", () => {
    render(
      <Transcript
        conversationID="project-a"
        state={conversation("History that must not flash")}
        loading
        onPrompt={vi.fn()}
      />,
    );

    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("Loading conversation…");
    expect(status).toHaveClass("items-center", "justify-center");
    expect(screen.queryByText("History that must not flash")).not.toBeInTheDocument();
    expect(screen.queryByText("What should the agent work on?")).not.toBeInTheDocument();
  });

  it("starts unseen history at the bottom and follows same-conversation updates only while pinned", () => {
    const scrollIntoView = vi.spyOn(Element.prototype, "scrollIntoView");
    const { rerender } = render(
      <Transcript
        conversationID="project-a"
        state={conversation("First")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    const scroll = screen.getByTestId("message-scroll");
    expect(scroll.scrollTop).toBe(800);

    contentHeight = 1_200;
    rerender(
      <Transcript
        conversationID="project-a"
        state={conversation("Second")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    expect(scroll.scrollTop).toBe(1_000);

    scroll.scrollTop = 300;
    fireEvent.scroll(scroll);
    contentHeight = 1_400;
    rerender(
      <Transcript
        conversationID="project-a"
        state={conversation("Third")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    expect(scroll.scrollTop).toBe(300);
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("restores each conversation's saved position and bottoms unseen history", () => {
    const { rerender } = render(
      <Transcript
        conversationID="project-a"
        state={conversation("Project A")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    const scroll = screen.getByTestId("message-scroll");
    scroll.scrollTop = 260;
    fireEvent.scroll(scroll);

    contentHeight = 700;
    rerender(
      <Transcript
        conversationID="project-b"
        state={conversation("Project B")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    expect(scroll.scrollTop).toBe(500);

    scroll.scrollTop = 90;
    fireEvent.scroll(scroll);
    contentHeight = 1_000;
    rerender(
      <Transcript
        conversationID="project-a"
        state={conversation("Project A")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    expect(scroll.scrollTop).toBe(260);

    contentHeight = 700;
    rerender(
      <Transcript
        conversationID="project-b"
        state={conversation("Project B")}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );
    expect(scroll.scrollTop).toBe(90);
  });

  it("renders web search citations as clickable links", () => {
    render(
      <Transcript
        conversationID="project-search"
        state={{
          ...emptyConversation,
          messages: [{
            id: "search-result",
            role: "tool",
            label: "web_search",
            content: "Search answer.\n\n![tracker](https://tracker.example/pixel.png)\n\nSources:\n- [OpenAI docs](https://developers.openai.com/api/docs/guides/tools-web-search)",
          }],
        }}
        loading={false}
        onPrompt={vi.fn()}
      />,
    );

    expect(screen.getByRole("link", { name: "OpenAI docs" }))
      .toHaveAttribute("href", "https://developers.openai.com/api/docs/guides/tools-web-search");
    expect(screen.getByRole("link", { name: "OpenAI docs" }))
      .toHaveAttribute("target", "_blank");
    expect(screen.getByRole("link", { name: "OpenAI docs" }))
      .toHaveAttribute("rel", "noreferrer");
    expect(screen.queryByRole("img", { name: "tracker" })).not.toBeInTheDocument();
  });
});
