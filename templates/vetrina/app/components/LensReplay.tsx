"use client";

// rrweb session replay, streamed to /t/rec - the heavier sibling of LensTracker
// Consent-gated: signed-in customers always record, anonymous ones only inside the
// server's sample %. Inputs masked at source; .lens-mask hides text, .lens-block blanks
// Session id + chunk seq live in sessionStorage so recording survives same-tab nav
// Private pages (recording-policy) pause it; the next public page restarts it with a fresh full snapshot

import { usePathname } from "next/navigation";
import { useEffect, useRef } from "react";

import { redactPath } from "@skelet/storefront-next/analytics/redact";
import { RECORDING_BLOCK_CLASS, isPrivateRecordingPath } from "@skelet/storefront-next/analytics/recording-policy";
import { ensureAnonId, sampleBucket, viewportBucket } from "@skelet/storefront-next/analytics/visitor";
import { randomId } from "@skelet/storefront-next/random-id";
import { beaconFetch } from "@skelet/storefront-next/beacons/client";
import { BEACON } from "@skelet/storefront-next/beacons/paths";

const SID_KEY = "lens_sid";
const SEQ_KEY = "lens_seq";
const STARTED_KEY = "lens_started"; // epoch ms the session's recording began (for the max-length cap)
const FLUSH_MS = 5000;
const FLUSH_EVENTS = 50; // flush early once the buffer reaches this many events
const CHECKOUT_MS = 2 * 60 * 1000; // periodic full snapshot → seekable + self-healing

// isNoiseInput drops rrweb input events that aren't real user input: programmatic value-sets fired on hydration/render
// (hidden form fields - SKUs, prices, checkboxes, locale codes) and inputs on nodes rrweb couldn't map (id < 0)
function isNoiseInput(ev: unknown): boolean {
  const e = ev as { type?: number; data?: { source?: number; userTriggered?: boolean; id?: number } };
  if (e.type !== 3 || e.data?.source !== 5) return false;
  return e.data?.userTriggered !== true || (typeof e.data?.id === "number" && e.data.id < 0);
}

// rrweb stamps the full location.href on every Meta event (one per full snapshot), which
// on /account/reset/<token> is the live secret itself - rewrite it to the redacted key
function scrubMetaHref(ev: unknown): unknown {
  const e = ev as { type?: number; data?: { href?: string } };
  if (e.type !== 4 || typeof e.data?.href !== "string") return ev;
  try {
    const u = new URL(e.data.href);
    e.data.href = u.origin + redactPath(u.pathname);
  } catch {
    e.data.href = "";
  }
  return ev;
}

function ensureSessionId(): string {
  let sid = sessionStorage.getItem(SID_KEY);
  if (!sid) {
    sid = randomId();
    sessionStorage.setItem(SID_KEY, sid);
  }
  return sid;
}

// sessionStartedAt returns (and lazily stamps) the epoch ms this session's recording began - persisted so the
// max-length cap is enforced across navigations + reloads, not just within one page
function sessionStartedAt(): number {
  const v = parseInt(sessionStorage.getItem(STARTED_KEY) || "0", 10);
  if (v > 0) return v;
  const now = Date.now();
  sessionStorage.setItem(STARTED_KEY, String(now));
  return now;
}

function nextSeq(): number {
  const cur = parseInt(sessionStorage.getItem(SEQ_KEY) || "0", 10) || 0;
  sessionStorage.setItem(SEQ_KEY, String(cur + 1));
  return cur;
}

export function LensReplay({
  enabled,
  channelId,
  customerId,
}: {
  enabled: boolean;
  channelId: string;
  customerId?: string;
}) {
  const pathname = usePathname();
  const isPrivate = isPrivateRecordingPath(pathname);
  const privateRef = useRef(isPrivate);
  // Set once rrweb is loaded and this session records - route changes pause and resume through it
  const toggleRef = useRef<((isPrivate: boolean) => void) | null>(null);

  useEffect(() => {
    privateRef.current = isPrivate;
    toggleRef.current?.(isPrivate);
  }, [isPrivate]);

  useEffect(() => {
    if (!enabled || !channelId || typeof window === "undefined") return;

    const anonId = ensureAnonId();
    let buffer: unknown[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let capTimer: ReturnType<typeof setTimeout> | null = null;
    let stopRecording: (() => void) | undefined;
    let cancelled = false;

    const flush = (keepalive: boolean) => {
      if (!buffer.length) return;
      const events = buffer;
      buffer = [];
      const body = JSON.stringify({
        sessionId: ensureSessionId(),
        anonId,
        customerId: customerId ?? "",
        seq: nextSeq(),
        page: redactPath(location.pathname),
        viewport: viewportBucket(),
        events,
      });
      // Best-effort beacon: a network failure is an async rejection (not a sync throw), so swallow it with .catch -
      // otherwise it leaks as an unhandled rejection (e.g. when the page is unloading or the backend is briefly down)
      void beaconFetch(BEACON.replay, {
        method: "POST",
        channelId,
        headers: { "Content-Type": "application/json" },
        body,
        keepalive,
      }).catch(() => {});
    };
    const schedule = () => {
      if (flushTimer) return;
      flushTimer = setTimeout(() => {
        flushTimer = null;
        flush(false);
      }, FLUSH_MS);
    };

    const onHide = () => {
      if (document.visibilityState === "hidden") flush(true);
    };
    const onPageHide = () => flush(true);

    void (async () => {
      let replayEnabled = true;
      let rate = 10;
      let maxMinutes = 30;
      try {
        const r = await beaconFetch(BEACON.config, { channelId });
        if (r.ok) {
          const c = (await r.json()) as { replayEnabled?: boolean; replaySamplingRate?: number; replayMaxMinutes?: number };
          replayEnabled = c.replayEnabled !== false;
          if (typeof c.replaySamplingRate === "number") rate = c.replaySamplingRate;
          if (typeof c.replayMaxMinutes === "number") maxMinutes = c.replayMaxMinutes;
        }
      } catch {
        /* keep permissive defaults on a config miss */
      }
      if (cancelled) return;
      // Logged-in customers are always recorded; anonymous visitors are sampled
      const active = replayEnabled && (!!customerId || sampleBucket(anonId) < rate);
      if (!active) return;

      // The bulk of this component's weight - fetched only for a session that will actually record
      let record: typeof import("rrweb").record;
      try {
        ({ record } = await import("rrweb"));
      } catch {
        return;
      }
      if (cancelled) return;

      const maxMs = maxMinutes > 0 ? maxMinutes * 60_000 : 0;
      let capped = false;
      if (maxMs > 0) {
        const elapsed = Date.now() - sessionStartedAt();
        if (elapsed >= maxMs) return;
        capTimer = setTimeout(() => {
          capped = true;
          stopRecording?.();
          stopRecording = undefined;
          flush(true);
        }, maxMs - elapsed);
      }

      // Every record() call opens with a Meta + full snapshot, so a resumed recording never patches onto a DOM the
      // replay last saw before the private page
      const start = () => {
        if (stopRecording || capped) return;
        stopRecording = record({
          emit(event) {
            if (isNoiseInput(event)) return; // drop programmatic/hidden-field input churn
            buffer.push(scrubMetaHref(event));
            if (buffer.length >= FLUSH_EVENTS) flush(false);
            else schedule();
          },
          maskAllInputs: true,
          maskTextClass: "lens-mask",
          blockClass: RECORDING_BLOCK_CLASS,
          recordCanvas: false,
          collectFonts: false,
          inlineStylesheet: true, // self-contained so the dashboard can replay cross-origin
          sampling: { mousemove: 50, scroll: 150, media: 800, input: "last" },
          userTriggeredOnInput: true, // flag real user input vs. programmatic value-sets
          checkoutEveryNms: CHECKOUT_MS,
        });
      };
      toggleRef.current = (nowPrivate) => {
        if (!nowPrivate) return start();
        stopRecording?.();
        stopRecording = undefined;
        flush(false);
      };
      toggleRef.current(privateRef.current);

      document.addEventListener("visibilitychange", onHide);
      window.addEventListener("pagehide", onPageHide);
    })();

    return () => {
      cancelled = true;
      toggleRef.current = null;
      document.removeEventListener("visibilitychange", onHide);
      window.removeEventListener("pagehide", onPageHide);
      if (flushTimer) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      if (capTimer) {
        clearTimeout(capTimer);
        capTimer = null;
      }
      stopRecording?.();
      flush(true);
    };
    // One-shot per (enabled, channelId); recording spans SPA navigations
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, channelId]);

  return null;
}
