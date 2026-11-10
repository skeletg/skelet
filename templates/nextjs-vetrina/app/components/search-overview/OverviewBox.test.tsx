import { act, fireEvent, screen } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { localeLinkMock } from "@/app/test/locale-link-mock";
import { renderWithIntl } from "@/app/test/render";
import type { Exchange } from "@/app/components/assistant/turns";

vi.mock("@/app/components/LocaleLink", () => localeLinkMock);
const cards = vi.hoisted(() => vi.fn());
vi.mock("@skelet/storefront-next/search/actions", () => ({ overviewCardsAction: cards }));

const { AssistantProvider, useAssistant } = await import("@/app/components/assistant/AssistantProvider");
const { OverviewBox, OverviewPending, OverviewSlot } = await import("./OverviewBox");

const dress = { id: "p1", name: "Smocked Midi Dress", url: "shop/dresses/smocked-midi-dress" };
const skirt = { id: "p2", name: "Linen Wrap Skirt", url: "shop/skirts/linen-wrap-skirt" };
const overview = { text: "For a wedding, [[1]] is airy and [[2]] ties at the waist. [[5]]", recap: "Shown as cards: two dresses", products: [dress, skirt] };
const chat = { question: "summer wedding", reply: "For a wedding, Smocked Midi Dress is airy.", recap: "Shown as cards: Smocked Midi Dress (id p1)", productIds: ["p1"] };
const dressCard = { id: "p1", name: "Smocked Midi Dress", kind: "SIMPLE", defaultPrice: 89 } as unknown as Exchange["products"][number];

const track = vi.fn();
beforeEach(() => {
  window.lens = { track };
  document.cookie = "storefront_consent=all; path=/";
});
afterEach(() => {
  delete window.lens;
  delete window.__lensQueue;
  document.cookie = "storefront_consent=; max-age=0; path=/";
  vi.clearAllMocks();
});

function Listener({ take }: { take: (e: Exchange) => void }) {
  const { onContinue } = useAssistant();
  useEffect(() => {
    onContinue(take);
    return () => onContinue(null);
  }, [onContinue, take]);
  return null;
}

describe("OverviewBox", () => {
  it("names each product the overview points at as a link to its page, and drops a marker that points nowhere", () => {
    const { container } = renderWithIntl(
      <AssistantProvider enabled>
        <OverviewBox overview={overview} chat={chat} />
      </AssistantProvider>,
    );
    expect(screen.getByRole("heading", { name: "AI overview" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Smocked Midi Dress" })).toHaveAttribute("href", "/shop/dresses/smocked-midi-dress");
    expect(screen.getByRole("link", { name: "Linen Wrap Skirt" })).toHaveAttribute("href", "/shop/skirts/linen-wrap-skirt");
    expect(container.querySelector("p")?.textContent).toBe("For a wedding, Smocked Midi Dress is airy and Linen Wrap Skirt ties at the waist.");
    expect(screen.getByText(/Written by AI from the products below/)).toBeInTheDocument();
    expect(window.__lensQueue).toEqual([{ name: "search_overview_shown", props: { query: "summer wedding", products: 2 } }]);
  });

  it("counts an overview shown once per search, not once per render, and never the placeholder", () => {
    const { rerender } = renderWithIntl(
      <AssistantProvider enabled>
        <OverviewPending />
      </AssistantProvider>,
    );
    expect(window.__lensQueue ?? []).toEqual([]);
    const at = (c: typeof chat) => (
      <AssistantProvider enabled>
        <OverviewBox overview={overview} chat={c} />
      </AssistantProvider>
    );
    rerender(at(chat));
    rerender(at({ ...chat }));
    expect(window.__lensQueue).toHaveLength(1);
    rerender(at({ ...chat, question: "beach wedding" }));
    expect(window.__lensQueue?.map((e) => e.props?.query)).toEqual(["summer wedding", "beach wedding"]);
  });

  it("tracks which product link was followed by its place in the overview", () => {
    renderWithIntl(
      <AssistantProvider enabled>
        <OverviewBox overview={overview} chat={chat} />
      </AssistantProvider>,
    );
    fireEvent.click(screen.getByRole("link", { name: "Linen Wrap Skirt" }));
    expect(track).toHaveBeenCalledWith("search_overview_click", { query: "summer wedding", position: 2 });
  });

  it("carries the exchange into the chat with its cards, fetched once on the way to the button", async () => {
    const take = vi.fn();
    cards.mockResolvedValue([dressCard]);
    renderWithIntl(
      <AssistantProvider enabled>
        <OverviewBox overview={overview} chat={chat} />
        <Listener take={take} />
      </AssistantProvider>,
    );
    const button = screen.getByRole("button", { name: "Continue in chat" });
    fireEvent.pointerEnter(button);
    await act(async () => fireEvent.click(button));
    expect(cards).toHaveBeenCalledTimes(1);
    expect(cards).toHaveBeenCalledWith(["p1"]);
    expect(take).toHaveBeenCalledWith({ question: chat.question, reply: chat.reply, recap: chat.recap, products: [dressCard] } satisfies Exchange);
    expect(track).toHaveBeenCalledWith("search_overview_chat", { query: "summer wedding" });
  });

  it("still carries the words on when the cards can't be fetched, and asks for them again next time", async () => {
    const take = vi.fn();
    cards.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce([dressCard]);
    renderWithIntl(
      <AssistantProvider enabled>
        <OverviewBox overview={overview} chat={chat} />
        <Listener take={take} />
      </AssistantProvider>,
    );
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Continue in chat" })));
    expect(take).toHaveBeenLastCalledWith(expect.objectContaining({ question: "summer wedding", products: [] }));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Continue in chat" })));
    expect(take).toHaveBeenLastCalledWith(expect.objectContaining({ products: [dressCard] }));
    expect(cards).toHaveBeenCalledTimes(2);
  });

  it("offers no chat on a store without the assistant", () => {
    renderWithIntl(
      <AssistantProvider enabled={false}>
        <OverviewBox overview={overview} chat={chat} />
      </AssistantProvider>,
    );
    expect(screen.getByRole("link", { name: "Smocked Midi Dress" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Continue in chat" })).toBeNull();
  });
});

describe("OverviewPending", () => {
  it("says it's writing, as a busy region under the same label", () => {
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    renderWithIntl(<OverviewPending />);
    const region = screen.getByRole("region", { name: "AI overview" });
    expect(region).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("Writing an overview…")).toBeInTheDocument();
  });
});

describe("OverviewSlot", () => {
  it("keeps its place when nothing comes, easing its room to whatever it holds", () => {
    let fit = () => {};
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(cb: () => void) {
          fit = cb;
        }
        observe() {}
        disconnect() {}
      },
    );
    const { container, rerender } = renderWithIntl(<OverviewSlot>{<p style={{ height: 200 }}>Writing</p>}</OverviewSlot>);
    const slot = container.querySelector(".search-overview-slot") as HTMLElement;
    const held = slot.firstElementChild as HTMLElement;
    Object.defineProperty(held, "offsetHeight", { configurable: true, get: () => (held.textContent ? 200 : 0) });
    act(() => fit());
    expect(slot.style.height).toBe("200px");
    rerender(<OverviewSlot>{null}</OverviewSlot>);
    act(() => fit());
    expect(container.querySelector(".search-overview-slot")).toBe(slot);
    expect(slot.style.height).toBe("0px");
    expect(slot).toHaveAttribute("aria-live", "polite");
    vi.unstubAllGlobals();
  });
});
