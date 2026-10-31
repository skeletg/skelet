"use client";

import type { snapshot } from "rrweb-snapshot";
import { usePathname } from "next/navigation";

import { useEffect, useRef } from "react";

import { redactPath } from "@/app/lib/analytics/redact";
import { RECORDING_BLOCK_CLASS, isPrivateRecordingPath } from "@/app/lib/analytics/recording-policy";
import { ensureAnonId, fnv1a, sampleBucket, viewportBucket } from "@/app/lib/analytics/visitor";
import { randomId } from "@skelet/storefront-next/random-id";
import { beaconFetch } from "@/app/lib/beacons/client";
import { BEACON } from "@/app/lib/beacons/paths";

const FLUSH_MS = 5000;
const MOVE_THROTTLE_MS = 50;

type LensEvent = {
  id: string;
  type: string;
  pageKey: string;
  viewport: string;
  title?: string;
  referrer?: string;
  x?: number;
  y?: number;
  depth?: number;
  selector?: string;
  text?: string;
  state?: string;
  name?: string;
  customerId?: string;
  props?: Record<string, unknown>;
  layoutHash?: string;
  html?: string;
  width?: number;
  height?: number;
  ts: number;
};

declare global {
  interface Window {
    lens?: {
      track: (name: string, props?: Record<string, unknown>) => void;
      flushQueue?: () => void;
      // Force-send the queued batch now (used for conversions, which shouldn't wait for
      // the 5s timer so the dashboard reflects them promptly)
      flush?: () => void;
    };
    __lensQueue?: Array<{ name: string; props?: Record<string, unknown> }>;
  }
}

function interactionFor(target: Element | null): { name: string; props: Record<string, unknown> } | null {
  const trigger = target?.closest("[data-lens-event],[aria-expanded]");
  if (!trigger) return null;
  const explicit = trigger.getAttribute("data-lens-event");
  if (explicit) return { name: explicit, props: {} };
  const action = trigger.getAttribute("aria-expanded") === "false" ? "expand" : "collapse";
  const label =
    (trigger.getAttribute("aria-label") || trigger.textContent || "")
      .replace(/\s+/g, " ")
      .trim()
      .slice(0, 60) || "toggle";
  return { name: label, props: { action } };
}

type State = {
  active: boolean;
  ready: boolean;
  captureMoves: boolean;
  anonId: string;
  channelId: string;
  curPageKey: string;
  curViewport: string;
  currentState: string; // active UI state ('default' | sidebar/modal key)
  seenLayouts: Set<string>; // layouts snapshotted this page-load (in-memory dedup)
  maxDepth: number;
  lastMove: number;
  queue: LensEvent[];
  flushTimer: ReturnType<typeof setTimeout> | null;
  flush?: (keepalive: boolean) => void;
  schedule?: () => void;
  // Loaded only once this visitor is sampled in - rrweb-snapshot is too heavy to put on every page
  snapshot?: typeof snapshot;
};

const OVERLAY_SEL =
  '[role="dialog"],[aria-modal="true"],[data-state="open"],[data-lens-overlay]';

function stateKeyOf(el: Element): string {
  const raw =
    el.getAttribute("data-lens-state") ||
    el.getAttribute("aria-label") ||
    el.getAttribute("role") ||
    el.tagName.toLowerCase();
  return raw.replace(/\s+/g, "-").toLowerCase().slice(0, 40) || "overlay";
}
function detectState(): string {
  let chosen: Element | null = null;
  document.querySelectorAll(OVERLAY_SEL).forEach((el) => {
    if (el.getAttribute("data-state") === "closed") return;
    if (el.getAttribute("aria-hidden") === "true") return;
    const he = el as HTMLElement;
    if (he.offsetParent === null && getComputedStyle(el).position !== "fixed") return;
    chosen = el; // later match in DOM order ≈ topmost overlay
  });
  return chosen ? stateKeyOf(chosen) : "default";
}

function selectorFor(el: Element | null): string {
  if (!el) return "";
  const parts: string[] = [];
  let cur: Element | null = el;
  let depth = 0;
  while (cur && cur.nodeType === 1 && depth < 3) {
    if (cur.id) {
      parts.unshift(`#${cur.id}`);
      break;
    }
    let part = cur.tagName.toLowerCase();
    if (cur.classList.length) {
      part += "." + Array.from(cur.classList).slice(0, 2).join(".");
    }
    parts.unshift(part);
    cur = cur.parentElement;
    depth++;
  }
  return parts.join(">").slice(0, 200);
}

function recordSnapshot(st: State, state: string) {
  const layoutKey = `${st.curPageKey}|${st.curViewport}|${state}`;
  if (st.seenLayouts.has(layoutKey) || !st.snapshot) return;
  // The live location, not curPageKey: a delayed overlay snapshot can fire after navigating onto a private page
  if (isPrivateRecordingPath(location.pathname)) return;
  try {
    // Same masking contract as LensReplay - without it the snapshot captures what the
    // recorder is told to skip (hidden token fields, .lens-block regions)
    const node = st.snapshot(document, {
      maskAllInputs: true,
      maskTextClass: "lens-mask",
      blockClass: RECORDING_BLOCK_CLASS,
      inlineStylesheet: true,
      slimDOM: false,
    });
    if (node) {
      const html = JSON.stringify(node);
      st.queue.push({
        id: randomId(),
        type: "snapshot",
        pageKey: st.curPageKey,
        viewport: st.curViewport,
        state,
        layoutHash: fnv1a(html).toString(16),
        html,
        width: document.documentElement.scrollWidth,
        height: document.documentElement.scrollHeight,
        ts: Date.now(),
      });
      st.seenLayouts.add(layoutKey);
      st.schedule?.();
    }
  } catch {
    /* snapshot is best-effort; page views + heat still flow without it */
  }
}

function pageContext(): Record<string, unknown> | undefined {
  const el = document.querySelector("[data-lens-entity]") as HTMLElement | null;
  if (!el) return undefined;
  const d = el.dataset;
  if (!d.lensEntity) return undefined;
  const props: Record<string, unknown> = { entity: d.lensEntity };
  if (d.lensName) props.entity_name = d.lensName;
  if (d.lensImage) props.entity_image = d.lensImage;
  if (d.lensPrice) props.entity_price = d.lensPrice;
  if (d.lensSku) props.sku = d.lensSku;
  if (d.lensAttrs) {
    try {
      const attrs = JSON.parse(d.lensAttrs) as Record<string, string>;
      for (const [slug, val] of Object.entries(attrs)) {
        if (val) props["attr:" + slug] = val;
      }
    } catch {
      /* malformed - skip affinities */
    }
  }
  return props;
}
function externalReferrer(): string {
  const r = document.referrer;
  if (!r) return "";
  try {
    return new URL(r).host === location.host ? "" : r;
  } catch {
    return r;
  }
}

function pageProps(): Record<string, unknown> | undefined {
  const props: Record<string, unknown> = { ...(pageContext() ?? {}) };
  try {
    const q = new URL(location.href).searchParams;
    const add = (key: string, prop: string) => {
      const v = q.get(key);
      if (v) props[prop] = v.slice(0, 200);
    };
    add("utm_source", "utm_source");
    add("utm_medium", "utm_medium");
    add("utm_campaign", "utm_campaign");
    add("q", "search_query");
  } catch {
  }
  return Object.keys(props).length ? props : undefined;
}

function recordPage(st: State, path: string) {
  st.curPageKey = redactPath(path);
  st.curViewport = viewportBucket();
  st.currentState = "default";
  st.queue.push({
    id: randomId(),
    type: "page_view",
    pageKey: st.curPageKey,
    viewport: st.curViewport,
    title: document.title,
    referrer: externalReferrer(),
    props: pageProps(),
    ts: Date.now(),
  });
  recordSnapshot(st, "default");
  st.schedule?.();
}

function enqueueIdentify(st: State, customerId: string) {
  st.queue.push({
    id: randomId(),
    type: "identify",
    pageKey: st.curPageKey,
    viewport: st.curViewport,
    customerId,
    ts: Date.now(),
  });
  st.schedule?.();
}

function flushScroll(st: State) {
  if (st.maxDepth <= 0 || !st.curPageKey) return;
  st.queue.push({
    id: randomId(),
    type: "scroll",
    pageKey: st.curPageKey,
    viewport: st.curViewport,
    state: st.currentState,
    depth: st.maxDepth,
    ts: Date.now(),
  });
  st.maxDepth = 0;
}

export function LensTracker({
  enabled,
  channelId,
  customerId,
}: {
  enabled: boolean;
  channelId: string;
  customerId?: string;
}) {
  // The REAL url including any locale prefix - analytics must not merge /de/x and /x into
  // one page, or the language dimension disappears from every Lens report
  const pathname = usePathname();
  const ref = useRef<State>({
    active: false,
    ready: false,
    captureMoves: true,
    anonId: "",
    channelId: "",
    curPageKey: "",
    curViewport: "",
    currentState: "default",
    seenLayouts: new Set(),
    maxDepth: 0,
    lastMove: 0,
    queue: [],
    flushTimer: null,
  });

  useEffect(() => {
    if (!enabled || !channelId || typeof window === "undefined") return;
    const st = ref.current;
    st.channelId = channelId;
    st.anonId = ensureAnonId();

    st.flush = (keepalive: boolean) => {
      if (!st.queue.length) return;
      const batch = {
        anonId: st.anonId,
        events: st.queue.splice(0, st.queue.length),
      };
      // Best-effort beacon: a network failure rejects the fetch promise asynchronously (it is not a synchronous throw), so .catch swallows
      // it - a bare `void fetch(...)` leaks an unhandled rejection when the page is unloading or the backend is briefly unreachable
      void beaconFetch(BEACON.collect, {
        method: "POST",
        channelId: st.channelId,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(batch),
        keepalive,
      }).catch(() => {});
    };
    st.schedule = () => {
      if (st.flushTimer) return;
      st.flushTimer = setTimeout(() => {
        st.flushTimer = null;
        st.flush?.(false);
      }, FLUSH_MS);
    };

    const enqueueCustom = (name: string, props: Record<string, unknown>) => {
      st.queue.push({
        id: randomId(),
        type: "custom",
        name,
        pageKey: st.curPageKey,
        viewport: st.curViewport,
        props,
        ts: Date.now(),
      });
      st.schedule?.();
    };

    let stateTimer: ReturnType<typeof setTimeout> | null = null;
    const checkState = () => {
      const ns = detectState();
      if (ns === st.currentState) return;
      st.currentState = ns;
      if (ns !== "default") {
        setTimeout(() => {
          if (st.active && st.currentState === ns) recordSnapshot(st, ns);
        }, 450);
      }
    };
    const scheduleStateCheck = () => {
      if (stateTimer) return;
      stateTimer = setTimeout(() => {
        stateTimer = null;
        checkState();
      }, 150);
    };

    // --- UX signals (frustration / errors / web vitals) --- Signals are emitted as reserved custom events ($rage_click
    // etc.) carrying the replay session id, so the backend can tie them to a recording
    const sigSessionId = (): string => {
      try {
        return sessionStorage.getItem("lens_sid") || "";
      } catch {
        return "";
      }
    };
    let lastMutationAt = 0; // bumped by the DOM observer → dead-click detection
    const recentClicks: { t: number; x: number; y: number }[] = [];
    let lastRageAt = 0;
    let errorCount = 0; // cap per page-load
    const onMutation = () => {
      lastMutationAt = Date.now();
      scheduleStateCheck();
    };
    const onError = (ev: ErrorEvent) => {
      if (!st.active || errorCount >= 10) return;
      errorCount++;
      enqueueCustom("$js_error", {
        message: String(ev.message || (ev.error && ev.error.message) || "error").slice(0, 300),
        source: String(ev.filename || "").slice(0, 200),
        line: ev.lineno || 0,
        sessionId: sigSessionId(),
      });
    };
    const onRejection = (ev: PromiseRejectionEvent) => {
      if (!st.active || errorCount >= 10) return;
      errorCount++;
      const r = ev.reason as { message?: string } | undefined;
      enqueueCustom("$js_error", {
        message: String(r?.message ?? r ?? "unhandledrejection").slice(0, 300),
        sessionId: sigSessionId(),
      });
    };

    const onClick = (e: MouseEvent) => {
      if (!st.active) return;
      const target = e.target as Element | null;
      st.currentState = detectState();
      st.queue.push({
        id: randomId(),
        type: "click",
        pageKey: st.curPageKey,
        viewport: st.curViewport,
        state: st.currentState,
        x: Math.round(e.pageX),
        y: Math.round(e.pageY),
        selector: selectorFor(target),
        text: (target?.textContent ?? "").trim().slice(0, 80),
        ts: Date.now(),
      });
      const ix = interactionFor(target);
      if (ix) enqueueCustom(ix.name, ix.props);
      st.schedule?.();

      const now = Date.now();
      const cx = Math.round(e.pageX);
      const cy = Math.round(e.pageY);
      recentClicks.push({ t: now, x: cx, y: cy });
      while (recentClicks.length && now - recentClicks[0].t > 1000) recentClicks.shift();
      const near = recentClicks.filter((c) => Math.abs(c.x - cx) <= 25 && Math.abs(c.y - cy) <= 25);
      if (near.length >= 3 && now - lastRageAt > 1000) {
        lastRageAt = now;
        enqueueCustom("$rage_click", {
          x: cx,
          y: cy,
          selector: selectorFor(target),
          text: (target?.textContent ?? "").trim().slice(0, 60),
          sessionId: sigSessionId(),
        });
      }
      const href = location.href;
      setTimeout(() => {
        if (st.active && lastMutationAt < now && location.href === href) {
          enqueueCustom("$dead_click", { x: cx, y: cy, selector: selectorFor(target), sessionId: sigSessionId() });
        }
      }, 700);

      setTimeout(checkState, 350);
    };

    window.lens = {
      track: (name: string, props?: Record<string, unknown>) => {
        if (st.active && name) enqueueCustom(name, props ?? {});
      },
      flushQueue: () => {
        if (!st.active) return;
        const q = window.__lensQueue;
        if (!q || q.length === 0) return;
        for (const ev of q.splice(0, q.length)) {
          if (ev && ev.name) enqueueCustom(ev.name, ev.props ?? {});
        }
      },
      flush: () => {
        if (st.active) st.flush?.(false);
      },
    };
    const onMove = (e: MouseEvent) => {
      if (!st.active || !st.captureMoves) return;
      const now = Date.now();
      if (now - st.lastMove < MOVE_THROTTLE_MS) return;
      st.lastMove = now;
      st.queue.push({
        id: randomId(),
        type: "move",
        pageKey: st.curPageKey,
        viewport: st.curViewport,
        state: st.currentState,
        x: Math.round(e.pageX),
        y: Math.round(e.pageY),
        ts: now,
      });
      st.schedule?.();
    };
    const onScroll = () => {
      if (!st.active) return;
      const doc = document.documentElement;
      const denom = doc.scrollHeight - doc.clientHeight;
      const depth =
        denom > 0
          ? Math.min(100, Math.round(((doc.scrollTop + doc.clientHeight) / doc.scrollHeight) * 100))
          : 100;
      if (depth > st.maxDepth) st.maxDepth = depth;
    };
    const onHide = () => {
      if (document.visibilityState === "hidden") {
        flushScroll(st);
        st.flush?.(true);
      }
    };
    const onPageHide = () => {
      flushScroll(st);
      st.flush?.(true);
    };
    window.addEventListener("click", onClick, { capture: true });
    window.addEventListener("mousemove", onMove, { passive: true });
    window.addEventListener("scroll", onScroll, { passive: true });
    document.addEventListener("visibilitychange", onHide);
    window.addEventListener("pagehide", onPageHide);

    const mo = new MutationObserver(onMutation);
    mo.observe(document.documentElement, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ["data-state", "aria-modal", "aria-hidden", "role", "hidden", "open"],
    });

    let cancelled = false;
    void (async () => {
      let rate = 100;
      let on = true;
      let moves = true;
      try {
        const r = await beaconFetch(BEACON.config, { channelId });
        if (r.ok) {
          const c = (await r.json()) as {
            enabled?: boolean;
            samplingRate?: number;
            captureMoves?: boolean;
          };
          on = c.enabled !== false;
          if (typeof c.samplingRate === "number") rate = c.samplingRate;
          moves = c.captureMoves !== false;
        }
      } catch {
        /* keep permissive defaults on a config miss */
      }
      if (cancelled) return;
      const sampled = on && sampleBucket(st.anonId) < rate;
      // Consented and sampled in, so now worth the download - a failed chunk load still leaves page
      // views and heat flowing, just without layout snapshots and vitals
      const [snap, vitals] = sampled
        ? await Promise.all([import("rrweb-snapshot").catch(() => null), import("web-vitals").catch(() => null)])
        : [null, null];
      if (cancelled) return;
      st.snapshot = snap?.snapshot;
      st.captureMoves = moves;
      st.ready = true;
      st.active = sampled;
      if (st.active) {
        recordPage(st, pathname);
        if (customerId) enqueueIdentify(st, customerId);
        window.lens?.flushQueue?.();
        window.addEventListener("error", onError);
        window.addEventListener("unhandledrejection", onRejection);
        const reportVital = (metric: string) => (m: { value: number; navigationType?: string }) => {
          if (m.navigationType === "back-forward-cache" || m.navigationType === "prerender" || m.navigationType === "restore") return;
          enqueueCustom("$web_vitals", { metric, value: Math.round(m.value * 1000) / 1000, sessionId: sigSessionId() });
        };
        if (vitals) {
          vitals.onLCP(reportVital("LCP"));
          vitals.onCLS(reportVital("CLS"));
          vitals.onINP(reportVital("INP"));
          vitals.onTTFB(reportVital("TTFB"));
        }
      }
    })();

    return () => {
      cancelled = true;
      window.removeEventListener("click", onClick, { capture: true });
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("scroll", onScroll);
      document.removeEventListener("visibilitychange", onHide);
      window.removeEventListener("pagehide", onPageHide);
      window.removeEventListener("error", onError);
      window.removeEventListener("unhandledrejection", onRejection);
      mo.disconnect();
      if (stateTimer) clearTimeout(stateTimer);
      if (st.flushTimer) {
        clearTimeout(st.flushTimer);
        st.flushTimer = null;
      }
      window.lens = undefined;
      flushScroll(st);
      st.flush?.(true);
    };
    // One-shot per (enabled, channelId); route changes handled below
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, channelId]);

  useEffect(() => {
    const st = ref.current;
    if (!st.ready || !st.active) return;
    flushScroll(st);
    recordPage(st, pathname);
  }, [pathname]);

  useEffect(() => {
    const st = ref.current;
    if (!st.ready || !st.active || !customerId) return;
    enqueueIdentify(st, customerId);
  }, [customerId]);

  return null;
}
