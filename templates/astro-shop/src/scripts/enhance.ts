// Progressive enhancement for the shop's plain forms. A form (or its submit button) marked data-enhance posts in
// the background and swaps in only the regions its selector list names, so a cart edit or a delivery pick doesn't
// reload the page. With scripts off the very same forms post and redirect as before.
//
//   data-enhance="[data-cart]"   regions to replace from the answer - empty swaps nothing (the add-to-cart form)
//   data-status / data-done      a form's live message: the answer's alert, else data-done
//   data-autosubmit="<id>"       a control that submits its form on change, through that submit button when named.
//                                On a number box it waits for the typing, arrows or +/- clicks to settle first
//   data-step="1" / "-1"         a button that steps its form's number box by the box's own step
import { popBadge } from "./motion";

const SETTLE_MS = 400;
// A form's pending settle, and forms whose shopper kept clicking while their last post was out
const timers = new Map<HTMLFormElement, ReturnType<typeof setTimeout>>();
const queued = new WeakSet<HTMLFormElement>();

function settle(form: HTMLFormElement) {
  clearTimeout(timers.get(form));
  timers.set(form, setTimeout(() => {
    timers.delete(form);
    form.requestSubmit();
  }, SETTLE_MS));
}

const isNumber = (el: Element): el is HTMLInputElement => el instanceof HTMLInputElement && el.type === "number";

export function initEnhance(doc: Document = document, win: Window = window): void {
  doc.addEventListener("submit", (e) => {
    const form = e.target as HTMLFormElement;
    const submitter = e.submitter as HTMLButtonElement | null;
    const regions = submitter?.dataset.enhance ?? form.dataset.enhance;
    if (regions === undefined || "native" in form.dataset) return;
    e.preventDefault();
    clearTimeout(timers.get(form));
    timers.delete(form);
    // One post at a time per form - a click that lands meanwhile goes out after it, from the box as it is then
    if (form.getAttribute("aria-busy") === "true") queued.add(form);
    else void send(doc, win, form, submitter, regions);
  });

  // Typing and the box's own arrows - an emptied box waits, it's someone about to retype
  doc.addEventListener("input", (e) => {
    const el = e.target as Element;
    if (isNumber(el) && el.form && el.matches("[data-autosubmit]") && el.value !== "") settle(el.form);
  });

  doc.addEventListener("change", (e) => {
    const el = e.target as HTMLInputElement;
    if (!el.form || !el.matches("[data-autosubmit]")) return;
    // A number box already settles on input - only one left empty is sent on leaving it, for the refusal
    if (isNumber(el) && el.value !== "") return;
    submit(el.form, el.dataset.autosubmit ? (doc.getElementById(el.dataset.autosubmit) as HTMLButtonElement) : null);
  });

  doc.addEventListener("click", (e) => {
    const btn = (e.target as Element).closest<HTMLButtonElement>("button[data-step]");
    const box = btn?.form?.querySelector<HTMLInputElement>("input[type=number]");
    if (!btn || !box) return;
    // step="any" steps by 1, as the box's own arrows do
    const size = Number(box.step) || 1;
    const next = Math.round(((Number(box.value) || 0) + size * Number(btn.dataset.step)) * 1000) / 1000;
    const max = box.max === "" ? Infinity : Number(box.max);
    box.value = String(Math.min(max, Math.max(Number(box.min || 0), next)));
    box.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function send(doc: Document, win: Window, form: HTMLFormElement, submitter: HTMLButtonElement | null, regions: string) {
  const selectors = regions.split(",").map((s) => s.trim()).filter(Boolean);
  const busy = [form, ...selectors.map((s) => doc.querySelector(s))].filter((el) => el !== null);
  for (const el of busy) el.setAttribute("aria-busy", "true");

  let res: Response;
  try {
    res = await fetch(form.action, { method: "POST", body: new FormData(form, submitter) });
  } catch {
    return native(form, submitter);
  }
  if (!res.ok) return native(form, submitter);
  const next = new DOMParser().parseFromString(await res.text(), "text/html");
  // The shopper changed the box again while this was out - this answer is already stale, so don't swap it in
  // over what they're doing; the newer post (queued, or still settling) brings the page that counts
  if (queued.has(form) || timers.has(form)) {
    for (const el of busy) el.removeAttribute("aria-busy");
    if (queued.delete(form)) form.requestSubmit();
    return;
  }
  const pairs = selectors.map((s) => [doc.querySelector(s), next.querySelector(s)] as const);
  // The answer is a different page (an emptied checkout goes back to the cart) - follow it there
  if (pairs.some(([now, then]) => !now || !then)) return win.location.assign(res.url);

  const status = form.querySelector("[data-status]");
  if (status) status.textContent = next.querySelector("[role=alert]")?.textContent?.trim() || form.dataset.done || "";

  const focused = doc.activeElement?.id;
  const apply = () => {
    for (const [now, then] of pairs) now!.replaceWith(doc.adoptNode(then!));
    syncBadge(doc);
  };
  const transition = !win.matchMedia("(prefers-reduced-motion: reduce)").matches && doc.startViewTransition?.(apply);
  if (transition) await transition.updateCallbackDone;
  else apply();
  for (const el of busy) el.removeAttribute("aria-busy");
  // The swapped-in controls carry the same ids, so the +/- the shopper was pressing keeps focus
  if (focused) doc.getElementById(focused)?.focus();
  popBadge(doc, win);
}

// The network failed or the server erred - submit for real and let the browser show what happened
function native(form: HTMLFormElement, submitter: HTMLButtonElement | null) {
  form.dataset.native = "";
  submit(form, submitter);
}

// requestSubmit(null) is allowed by the spec but refused by some engines (jsdom) - pass a submitter only when there is one
function submit(form: HTMLFormElement, via: HTMLButtonElement | null) {
  if (via) form.requestSubmit(via);
  else form.requestSubmit();
}

// Base.astro's inline script does the same read before first paint - repeated here because a background post
// changes the count without a page load
function syncBadge(doc: Document) {
  const n = doc.cookie.match(/(?:^|; )cart_count=(\d+)/)?.[1];
  const badge = doc.querySelector<HTMLElement>("[data-cart-count]");
  if (!badge) return;
  badge.textContent = n ?? "";
  badge.hidden = !n || n === "0";
}
