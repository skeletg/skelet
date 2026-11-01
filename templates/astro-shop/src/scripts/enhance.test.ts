// @vitest-environment jsdom
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("motion/mini", () => ({ animate: vi.fn(() => Promise.resolve()) }));
vi.mock("motion", () => ({ inView: vi.fn() }));

const { initEnhance } = await import("./enhance");

const fetchMock = vi.fn();
const assign = vi.fn();
const win = { matchMedia: () => ({ matches: false }), location: { assign } } as unknown as Window;

const cartPage = (qty: number, alert = "") => `
  <span data-cart-count hidden></span>
  <div data-cart>
    ${alert && `<p role="alert">${alert}</p>`}
    <form method="post" action="/cart" data-enhance="[data-cart]">
      <input type="hidden" name="lineId" value="l1" />
      <button type="button" id="more-l1" data-step="1">+</button>
      <input type="number" name="quantity" value="${qty}" min="0" step="any" data-autosubmit />
      <button type="submit">Update</button>
    </form>
  </div>`;

const answer = (html: string, url = "http://localhost/cart") => ({ ok: true, url, text: async () => html });
const settle = () => new Promise((r) => setTimeout(r, 0));

beforeAll(() => initEnhance(document, win));

beforeEach(() => {
  fetchMock.mockReset();
  assign.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  document.cookie = "cart_count=; Max-Age=0; Path=/";
});

afterEach(() => vi.useRealTimers());

describe("enhanced forms", () => {
  it("posts in the background and swaps in only the named region", async () => {
    document.body.innerHTML = cartPage(1);
    fetchMock.mockImplementation(async () => {
      document.cookie = "cart_count=2; Path=/";
      return answer(cartPage(2));
    });
    const form = document.querySelector("form")!;
    form.requestSubmit();
    await settle();

    expect(fetchMock).toHaveBeenCalledWith(form.action, expect.objectContaining({ method: "POST" }));
    expect(document.querySelector<HTMLInputElement>("input[name=quantity]")!.value).toBe("2");
    expect(document.querySelector("[data-cart-count]")!.textContent).toBe("2");
    expect(assign).not.toHaveBeenCalled();
  });

  it("lets rapid +/- clicks settle into one post", async () => {
    vi.useFakeTimers();
    document.body.innerHTML = cartPage(1);
    fetchMock.mockResolvedValue(answer(cartPage(4)));
    const more = document.getElementById("more-l1")!;
    more.click();
    more.click();
    more.click();
    expect(document.querySelector<HTMLInputElement>("input[name=quantity]")!.value).toBe("4");
    expect(fetchMock).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(450);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect((fetchMock.mock.calls[0]![1] as { body: FormData }).body.get("quantity")).toBe("4");
  });

  it("settles the box's own arrows and typing the same way", async () => {
    vi.useFakeTimers();
    document.body.innerHTML = cartPage(1);
    fetchMock.mockResolvedValue(answer(cartPage(3)));
    const box = document.querySelector<HTMLInputElement>("input[name=quantity]")!;
    for (const v of ["2", "3"]) {
      box.value = v;
      box.dispatchEvent(new Event("input", { bubbles: true }));
      box.dispatchEvent(new Event("change", { bubbles: true }));
    }
    await vi.advanceTimersByTimeAsync(450);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect((fetchMock.mock.calls[0]![1] as { body: FormData }).body.get("quantity")).toBe("3");
  });

  it("waits on an emptied box, and sends it only on leaving it", async () => {
    vi.useFakeTimers();
    document.body.innerHTML = cartPage(1);
    fetchMock.mockResolvedValue(answer(cartPage(1, "That didn't work")));
    const box = document.querySelector<HTMLInputElement>("input[name=quantity]")!;
    box.value = "";
    box.dispatchEvent(new Event("input", { bubbles: true }));
    await vi.advanceTimersByTimeAsync(450);
    expect(fetchMock).not.toHaveBeenCalled();
    box.dispatchEvent(new Event("change", { bubbles: true }));
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("queues clicks made while a post is out, and doesn't swap the stale answer over them", async () => {
    vi.useFakeTimers();
    document.body.innerHTML = cartPage(1);
    let release!: () => void;
    fetchMock.mockImplementationOnce(() => new Promise((r) => (release = () => r(answer(cartPage(2))))));
    fetchMock.mockResolvedValueOnce(answer(cartPage(4)));
    const box = () => document.querySelector<HTMLInputElement>("input[name=quantity]")!;
    document.getElementById("more-l1")!.click();
    await vi.advanceTimersByTimeAsync(450);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Still clicking while the first post is out
    document.getElementById("more-l1")!.click();
    document.getElementById("more-l1")!.click();
    await vi.advanceTimersByTimeAsync(450);
    release();
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect((fetchMock.mock.calls[1]![1] as { body: FormData }).body.get("quantity")).toBe("4");
    expect(box().value).toBe("4");
  });

  it("steps by the box's own step, inside its min and max, and posts nothing without autosubmit", async () => {
    vi.useFakeTimers();
    document.body.innerHTML = `
      <form method="post" action="/cart" data-enhance="">
        <button type="button" id="less" data-step="-1">-</button>
        <input type="number" name="quantity" value="2" min="2" max="6" step="2" />
        <button type="button" id="more" data-step="1">+</button>
      </form>`;
    const box = document.querySelector<HTMLInputElement>("input")!;
    document.getElementById("more")!.click();
    expect(box.value).toBe("4");
    document.getElementById("more")!.click();
    document.getElementById("more")!.click();
    expect(box.value).toBe("6");
    for (let i = 0; i < 4; i++) document.getElementById("less")!.click();
    expect(box.value).toBe("2");
    await vi.advanceTimersByTimeAsync(450);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("never steps below the box's minimum", () => {
    document.body.innerHTML = cartPage(0);
    document.querySelector<HTMLButtonElement>("[data-step]")!.dataset.step = "-1";
    document.querySelector<HTMLButtonElement>("[data-step]")!.click();
    expect(document.querySelector<HTMLInputElement>("input[name=quantity]")!.value).toBe("0");
  });

  it("shows the answer's alert, or the form's own done message, in its status line", async () => {
    document.body.innerHTML = `
      <form method="post" action="/cart" data-enhance="" data-done="Added to your cart.">
        <p data-status></p><button type="submit">Add</button>
      </form>`;
    const status = document.querySelector("[data-status]")!;
    fetchMock.mockResolvedValueOnce(answer(cartPage(1)));
    document.querySelector("form")!.requestSubmit();
    await settle();
    expect(status.textContent).toBe("Added to your cart.");

    fetchMock.mockResolvedValueOnce(answer(cartPage(1, "Only 2 left")));
    document.querySelector("form")!.requestSubmit();
    await settle();
    expect(status.textContent).toBe("Only 2 left");
  });

  it("follows the answer when it's a different page", async () => {
    document.body.innerHTML = cartPage(1);
    fetchMock.mockResolvedValue(answer("<p>Your cart is somewhere else</p>", "http://localhost/elsewhere"));
    document.querySelector("form")!.requestSubmit();
    await settle();
    expect(assign).toHaveBeenCalledWith("http://localhost/elsewhere");
  });

  it("leaves unmarked forms alone", () => {
    document.body.innerHTML = `<form method="post" action="/checkout"><button type="submit">Go</button></form>`;
    const form = document.querySelector("form")!;
    const native = vi.fn((e: Event) => e.preventDefault());
    form.addEventListener("submit", native);
    form.requestSubmit();
    expect(native).toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
