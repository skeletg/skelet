import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AssistantAnswer } from "@skelet/storefront-core/assistant/types";
import { NextIntlClientProvider } from "next-intl";

import messages from "@/messages/en.json";
import { renderWithIntl } from "@/app/test/render";

const mocks = vi.hoisted(() => ({ send: vi.fn(), page: vi.fn(async () => null), hydrate: vi.fn(), push: vi.fn(), openCart: vi.fn() }));
const router = { push: mocks.push };

vi.mock("@skelet/storefront-core/assistant/client", () => ({ sendToAssistant: mocks.send, readAssistantPage: mocks.page }));
vi.mock("@skelet/storefront-next/cart/store", () => ({ useCartStore: () => ({ hydrate: mocks.hydrate }) }));
vi.mock("@/app/components/CartUIProvider", () => ({ useCartUI: () => ({ open: mocks.openCart }) }));
vi.mock("@/app/components/ChannelProvider", () => ({ useChannel: () => ({ publicUrl: "https://shop.example" }) }));
vi.mock("@/app/lib/use-phone", () => ({ usePhone: () => false }));
vi.mock("@/app/components/ProductCard", () => ({ ProductCard: () => null }));
vi.mock("@/app/components/AddToCartForm", () => ({ AddToCartForm: () => null }));
vi.mock("@/app/components/Drawer", () => ({ Drawer: () => null }));
vi.mock("@/app/components/LocaleLink", () => ({
  Link: ({ href, children }: { href: string; children: ReactNode }) => <a href={href}>{children}</a>,
  usePathname: () => "/",
  useRouter: () => router,
}));

const { AssistantProvider, useAssistant } = await import("./AssistantProvider");
const { AssistantTrigger } = await import("./AssistantTrigger");
const { AssistantDock } = await import("./AssistantDock");
const { AskAboutButton } = await import("./AskAboutButton");
const { CONVERSATION_KEY } = await import("./turns");

// Each turn parks until the test answers it, so answers can land after whatever the test does in between
let pending: ((a: AssistantAnswer) => void)[] = [];
const answer = (over: Partial<Extract<AssistantAnswer, { ok: true }>> = {}): AssistantAnswer => ({
  ok: true,
  reply: "Sure",
  incomplete: false,
  recap: "",
  followUps: [],
  products: [],
  detail: null,
  links: [],
  navigate: null,
  cartChanged: false,
  checkout: false,
  ...over,
});

beforeEach(() => {
  pending = [];
  sessionStorage.clear();
  Element.prototype.scrollTo = vi.fn();
  mocks.send.mockImplementation(() => new Promise<AssistantAnswer>((done) => pending.push(done)));
  window.history.replaceState({}, "", "/");
});
afterEach(() => vi.clearAllMocks());

const overview = { question: "summer wedding", reply: "The Smocked Midi Dress is airy.", recap: "Shown as cards: Smocked Midi Dress (id p1)", products: [] };

function CarryOn() {
  const { continueWith } = useAssistant();
  return (
    <button type="button" onClick={() => continueWith(overview)}>
      Continue in chat
    </button>
  );
}

const dock = (name?: string) => (
  <AssistantProvider enabled name={name}>
    <AssistantTrigger />
    <AskAboutButton name="Enamel mug" />
    <CarryOn />
    <AssistantDock storeName="Workshop" />
  </AssistantProvider>
);
const carryOn = () => act(async () => fireEvent.click(screen.getByRole("button", { name: "Continue in chat" })));
const said = () => (JSON.parse(sessionStorage.getItem(CONVERSATION_KEY) ?? "{}").turns ?? []).map((t: { content: string }) => t.content);

const BOX = { name: "Ask about products, delivery or returns" };
const box = () => screen.getByRole("textbox", BOX) as HTMLTextAreaElement;

// the panel is its own chunk, and a busy machine can take longer than findBy's second to load it
async function renderDock(name?: string) {
  renderWithIntl(dock(name));
  fireEvent.click(screen.getByRole("button", { name: "Ask the shopping assistant" }));
  await screen.findByRole("textbox", BOX, { timeout: 5000 });
}
async function say(text: string) {
  fireEvent.change(box(), { target: { value: text } });
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Send" })));
}
const reply = async (a: AssistantAnswer) => act(async () => pending.shift()!(a));
// The reply's words show once on screen and once more in the live region that reads them out
const shown = (text: RegExp) => screen.queryAllByText(text).filter((el) => !el.closest(".sr-only"));
const announced = () => document.querySelector("[aria-live]")?.textContent ?? "";

describe("AssistantDock", () => {
  it("brings the tab's conversation back before saving anything over it", async () => {
    sessionStorage.setItem(CONVERSATION_KEY, JSON.stringify({ turns: [{ role: "user", content: "mugs?" }, { role: "assistant", content: "Here are two." }, { role: "robot", content: 7 }] }));
    await renderDock();
    expect(await screen.findByText("Here are two.")).toBeInTheDocument();
    expect(JSON.parse(sessionStorage.getItem(CONVERSATION_KEY)!).turns).toEqual([
      { role: "user", content: "mugs?" },
      { role: "assistant", content: "Here are two." },
    ]);
  });

  it("names the store in its intro", async () => {
    await renderDock();
    expect(await screen.findByText(/shopping at Workshop/)).toBeInTheDocument();
  });

  it("goes by the name the store gave it, and by the template's own words without one", async () => {
    await renderDock("Ava");
    expect(screen.getAllByText("Ava").length).toBeGreaterThan(0);
    expect(screen.queryByText("Shopping assistant")).toBeNull();
  });

  it("keeps one chat id across a reload, and a new one for New chat", async () => {
    await renderDock();
    await say("hi");
    const first = mocks.send.mock.calls[0][2].conversation as string;
    await reply(answer());
    expect(JSON.parse(sessionStorage.getItem(CONVERSATION_KEY)!).id).toBe(first);
    fireEvent.click(screen.getByRole("button", { name: "New chat" }));
    await say("again");
    expect(mocks.send.mock.calls[1][2].conversation).not.toBe(first);
  });

  it("drops an answer that lands after New chat", async () => {
    await renderDock();
    await say("hi");
    fireEvent.click(screen.getByRole("button", { name: "New chat" }));
    await reply(answer({ reply: "Late answer" }));
    expect(screen.queryByText("Late answer")).toBeNull();
    expect(screen.queryByText("hi")).toBeNull();
    expect(screen.getByText("Hi there")).toBeInTheDocument();
  });

  it("says what it's doing while the shopper waits, and stops when the answer lands", async () => {
    await renderDock();
    await say("rain jackets?");
    const onStep = mocks.send.mock.calls[0][2].onStep as (s: { tool: string; detail: string }) => void;
    expect(screen.getByText("Thinking…")).toBeTruthy();
    act(() => onStep({ tool: "search_products", detail: "rain jacket" }));
    expect(screen.getByText("Searching for “rain jacket”…")).toBeTruthy();
    act(() => onStep({ tool: "estimate_shipping", detail: "DE" }));
    expect(screen.getByText("Checking delivery to Germany…")).toBeTruthy();
    await reply(answer({ reply: "Here are two" }));
    expect(screen.queryByText(/Checking delivery/)).toBeNull();
  });

  it("moves the shopper's list only while they're still on the page that asked", async () => {
    await renderDock();
    const navigate = { label: "", path: "/shop/jackets", query: "", filters: [{ slug: "color", values: ["navy"] }], sort: "", count: 4 };
    await say("only navy");
    window.history.pushState({}, "", "/shop/rain-shell");
    await reply(answer({ navigate }));
    expect(mocks.push).not.toHaveBeenCalled();
    await say("only navy");
    await reply(answer({ navigate }));
    expect(mocks.push).toHaveBeenCalledWith("/shop/jackets/f/color/v/navy", { scroll: false });
  });

  it("hands a changed cart to the store, and offers the cart even when it couldn't be read again", async () => {
    await renderDock();
    const cart = { id: "c1" } as never;
    await say("two mugs");
    await reply(answer({ cartChanged: true, cart }));
    expect(mocks.hydrate).toHaveBeenCalledWith(cart);
    await say("and a third");
    await reply(answer({ cartChanged: true }));
    expect(mocks.hydrate).toHaveBeenCalledTimes(1);
    const buttons = screen.getAllByRole("button", { name: "Review cart" });
    expect(buttons).toHaveLength(2);
    fireEvent.click(buttons[1]);
    expect(mocks.openCart).toHaveBeenCalled();
  });

  it("says when what it added didn't reach the shopper's cart, all of it or the lines that stayed behind", async () => {
    await renderDock();
    await say("a mug and a cap");
    await reply(answer({ reply: "Added both", cartChanged: true, cartMissed: ["Wool cap"] }));
    expect(screen.getByText("These didn't make it into your cart: Wool cap.")).toBeInTheDocument();
    await say("and a scarf");
    await reply(answer({ reply: "Added it", cartChanged: true, cartLost: true }));
    expect(screen.getByText(/What I added didn't make it into your cart/)).toBeInTheDocument();
    expect(JSON.parse(sessionStorage.getItem(CONVERSATION_KEY)!).turns.at(-1)).toMatchObject({ cartLost: true });
  });

  it("says when its answer was cut short, and apologises only for a reply that showed nothing", async () => {
    await renderDock();
    await say("two mugs");
    await reply(answer({ reply: "", incomplete: true, cartChanged: true }));
    expect(shown(/My answer got cut short/)).toHaveLength(1);
    expect(shown(/couldn't put an answer together/)).toHaveLength(0);
    expect(announced()).toMatch(/My answer got cut short/);
    await say("hmm");
    await reply(answer({ reply: "" }));
    expect(shown(/couldn't put an answer together/)).toHaveLength(1);
    expect(announced()).toMatch(/couldn't put an answer together/);
  });

  it("gives a failed message back and offers to try it again", async () => {
    await renderDock();
    await say("hi");
    expect(box().value).toBe("");
    await reply({ ok: false, problem: "failed" });
    expect(box().value).toBe("hi");
    expect(screen.queryByText("hi", { selector: "p" })).toBeNull();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Try again" })));
    expect(mocks.send).toHaveBeenLastCalledWith([{ role: "user", content: "hi" }], "/", expect.objectContaining({ signal: expect.any(AbortSignal), onStep: expect.any(Function), conversation: expect.stringMatching(/^[0-9a-f-]{36}$/) }));
  });

  it("asks a message sent from the page while it's busy once the current one is answered", async () => {
    await renderDock();
    await say("first");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Tell me more" })));
    expect(mocks.send).toHaveBeenCalledTimes(1);
    await reply(answer({ reply: "One" }));
    expect(mocks.send).toHaveBeenCalledTimes(2);
    expect(mocks.send.mock.calls[1][0].at(-1)).toEqual({ role: "user", content: "Tell me about Enamel mug" });
  });

  it("closes on Escape and hands focus back to the header button", async () => {
    await renderDock();
    const trigger = screen.getByRole("button", { name: "Ask the shopping assistant" });
    await waitFor(() => expect(box()).toHaveFocus());
    fireEvent.keyDown(box(), { key: "Escape" });
    expect(trigger).toHaveFocus();
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveAttribute("aria-controls", "assistant-panel");
  });

  it("ships nothing and touches no storage until it's first opened, then keeps the chat across a close", async () => {
    renderWithIntl(dock());
    expect(screen.queryByRole("textbox", BOX)).toBeNull();
    expect(sessionStorage.getItem(CONVERSATION_KEY)).toBeNull();
    const trigger = screen.getByRole("button", { name: "Ask the shopping assistant" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(trigger);
    await screen.findByRole("textbox", BOX, { timeout: 5000 });
    await say("hi");
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(document.getElementById("assistant-panel")).not.toBeNull();
    fireEvent.click(trigger);
    await reply(answer({ reply: "Still here" }));
    fireEvent.click(trigger);
    expect(shown(/Still here/)).toHaveLength(1);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

  it("asks from the product page before the panel's code has loaded", async () => {
    renderWithIntl(dock());
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Tell me more" })));
    await screen.findByRole("textbox", BOX, { timeout: 5000 });
    await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(1));
    expect(mocks.send.mock.calls[0][0]).toEqual([{ role: "user", content: "Tell me about Enamel mug" }]);
  });

  it("gives the page its gutter while open, and takes it back on close", async () => {
    await renderDock();
    const root = document.documentElement;
    await waitFor(() => expect(root.dataset.assistantOpen).toBe(""));
    expect(root.style.getPropertyValue("--assistant-inset")).toBe("min(520px, var(--assistant-max-share))");
    fireEvent.keyDown(box(), { key: "Escape" });
    expect(root.dataset.assistantOpen).toBeUndefined();
    expect(root.style.getPropertyValue("--assistant-inset")).toBe("");
  });

  it("clears the box when a retry goes, and hands the words back if it fails again", async () => {
    await renderDock();
    await say("hi");
    await reply({ ok: false, problem: "failed" });
    expect(box().value).toBe("hi");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Try again" })));
    expect(box().value).toBe("");
    await reply({ ok: false, problem: "failed" });
    expect(box().value).toBe("hi");
    fireEvent.change(box(), { target: { value: "something else" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Try again" })));
    expect(box().value).toBe("something else");
    await reply(answer());
    expect(mocks.send).toHaveBeenCalledTimes(3);
  });

  it("never tells the assistant about the shopper's own pages, and sends only a page's search", async () => {
    await renderDock();
    window.history.replaceState({}, "", "/account/reset/secret-token?x=1");
    await say("hi");
    expect(mocks.send.mock.calls[0][1]).toBe("");
    await reply(answer());
    window.history.replaceState({}, "", "/search?q=mugs&utm_source=mail");
    await say("cheaper?");
    expect(mocks.send.mock.calls[1][1]).toBe("/search?q=mugs");
  });

  it("asks in the shopper's language", async () => {
    render(
      <NextIntlClientProvider locale="de" messages={messages}>
        {dock()}
      </NextIntlClientProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Ask the shopping assistant" }));
    await screen.findByRole("textbox", BOX, { timeout: 5000 });
    await waitFor(() => expect(mocks.page).toHaveBeenCalledWith("/", expect.objectContaining({ locale: "de" })));
    await say("hallo");
    expect(mocks.send.mock.calls[0][2]).toMatchObject({ locale: "de" });
  });

  it("brings a new reply in from its top rather than its end", async () => {
    await renderDock();
    await say("tell me everything");
    const rect = vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      return { top: this.dataset.turn === "1" ? 300 : 100 } as DOMRect;
    });
    await reply(answer({ reply: "A long answer" }));
    expect(Element.prototype.scrollTo).toHaveBeenLastCalledWith({ top: 184, behavior: "smooth" });
    rect.mockRestore();
  });

  it("carries a search's overview in before the panel's code has loaded, and asks the next turn on top of it", async () => {
    renderWithIntl(dock());
    await carryOn();
    await screen.findByRole("textbox", BOX, { timeout: 5000 });
    expect(await screen.findByText("summer wedding")).toBeInTheDocument();
    expect(shown(/Smocked Midi Dress is airy/)).toHaveLength(1);
    expect(announced()).toBe("The Smocked Midi Dress is airy.");
    expect(mocks.send).not.toHaveBeenCalled();
    await say("which is most formal?");
    expect(mocks.send.mock.calls[0][0]).toEqual([
      { role: "user", content: "summer wedding" },
      { role: "assistant", content: "The Smocked Midi Dress is airy.", recap: "Shown as cards: Smocked Midi Dress (id p1)" },
      { role: "user", content: "which is most formal?" },
    ]);
  });

  it("keeps an overview brought in while a turn is out until that turn is answered, and never repeats it", async () => {
    await renderDock();
    await say("hi");
    await carryOn();
    expect(screen.queryByText("summer wedding")).toBeNull();
    await reply(answer({ reply: "Hello" }));
    expect(said()).toEqual(["hi", "Hello", "summer wedding", "The Smocked Midi Dress is airy."]);
    await carryOn();
    expect(said()).toHaveLength(4);
  });

  it("drops an overview held for a turn that New chat threw away", async () => {
    await renderDock();
    await say("hi");
    await carryOn();
    fireEvent.click(screen.getByRole("button", { name: "New chat" }));
    await reply(answer());
    expect(said()).toEqual([]);
  });

  it("won't move the shopper off the store", async () => {
    await renderDock();
    await say("only navy");
    await reply(answer({ navigate: { label: "", path: "//evil.example", query: "", filters: [], sort: "", count: 4 } }));
    expect(mocks.push).not.toHaveBeenCalled();
  });
});
