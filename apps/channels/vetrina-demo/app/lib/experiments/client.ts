"use client";

// Prism - frontend A/B SDK. Fetches active experiments from /t/experiments, assigns the
// visitor deterministically from the Lens anon id (same FNV-1a bucketing as the Go
// backend, so client + server agree), renders the assigned variation's value, and fires
// a one-shot exposure through the Lens tracker (window.lens.track)

import { useEffect, useState } from "react";
import { randomId } from "@/app/lib/random-id";

type Variation = { key: string; weight: number; value: string };
type Experiment = { key: string; coverage: number; goalEvent: string; variations: Variation[] };

const AID_COOKIE = "lens_aid";

// The backend resolves the channel from X-Channel-Id (the /t proxy is a plain
// pass-through and can't inject it)
function channelId(): string {
  if (typeof document === "undefined") return "";
  return document.documentElement.dataset.channelId || "";
}

let cache: Promise<Experiment[]> | null = null;
function loadExperiments(): Promise<Experiment[]> {
  if (!cache) {
    cache = fetch("/t/experiments", { headers: { Accept: "application/json", "X-Channel-Id": channelId() } })
      .then((r) => (r.ok ? r.json() : []))
      .catch(() => [] as Experiment[]);
  }
  return cache;
}

function fnv1a(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function bucket(seed: string, anonID: string): number {
  return (fnv1a(seed + anonID) % 10000) / 10000;
}
function ensureAnonID(): string {
  // the guard belongs HERE, above the document.cookie read below it - and randomId covers the
  // non-secure-context case on its own
  if (typeof document === "undefined") return "";
  const m = document.cookie.match(new RegExp(`(?:^|; )${AID_COOKIE}=([^;]*)`));
  if (m) return decodeURIComponent(m[1]);
  const id = randomId();
  document.cookie = `${AID_COOKIE}=${id}; max-age=${60 * 60 * 24 * 365}; path=/; samesite=lax`;
  return id;
}

const FORCE_PARAM = "prism";
const FORCE_STORE = "prism_force";
function forcedVariations(): Record<string, string> {
  let store: Record<string, string> = {};
  try {
    store = JSON.parse(sessionStorage.getItem(FORCE_STORE) || "{}");
  } catch {
    store = {};
  }
  try {
    const raw = new URLSearchParams(window.location.search).get(FORCE_PARAM);
    if (raw === "off" || raw === "clear") {
      sessionStorage.removeItem(FORCE_STORE);
      return {};
    }
    if (raw) {
      for (const pair of raw.split(",")) {
        const [k, v] = pair.split(":");
        if (k && v) store[k.trim()] = v.trim();
      }
      sessionStorage.setItem(FORCE_STORE, JSON.stringify(store));
    }
  } catch {
    /* no window / storage access - ignore */
  }
  return store;
}

function assign(exp: Experiment, anonID: string): Variation | null {
  if (exp.coverage <= 0 || exp.variations.length === 0) return null;
  if (exp.coverage < 100 && bucket(`${exp.key}:coverage`, anonID) >= exp.coverage / 100) return null;
  const total = exp.variations.reduce((a, v) => a + Math.max(0, v.weight), 0);
  if (total <= 0) return exp.variations[0];
  const target = bucket(exp.key, anonID) * total;
  let acc = 0;
  for (const v of exp.variations) {
    acc += Math.max(0, v.weight);
    if (target < acc) return v;
  }
  return exp.variations[exp.variations.length - 1];
}

type LensGlobal = {
  track?: (name: string, props?: Record<string, unknown>) => void;
  flush?: () => void;
};

export type ClientAssignment = { variation: string; value: string };

// Assigns the visitor to a variation of `key` (null while loading or when not entered - fall back to your control).
// Callers that key off the operator-set value want useExperiment; those that match on the variation KEY want this
export function useExperimentAssignment(key: string): ClientAssignment | null {
  const [assignment, setAssignment] = useState<ClientAssignment | null>(null);
  useEffect(() => {
    let alive = true;
    loadExperiments().then((exps) => {
      if (!alive) return;
      const exp = exps.find((e) => e.key === key);
      if (!exp) return;

      // QA override: render a specific arm without firing an exposure
      const forced = forcedVariations()[key];
      if (forced) {
        const fv = exp.variations.find((v) => v.key === forced);
        if (fv) {
          setAssignment({ variation: fv.key, value: fv.value });
          return;
        }
      }

      const anon = ensureAnonID();
      if (!anon) return;
      const v = assign(exp, anon);
      if (!v) return;
      setAssignment({ variation: v.key, value: v.value });
      const lens = (window as unknown as { lens?: LensGlobal }).lens;
      lens?.track?.("$experiment", { experiment: key, variation: v.key });
    });
    return () => {
      alive = false;
    };
  }, [key]);
  return assignment;
}

export function useExperiment(key: string): string | null {
  return useExperimentAssignment(key)?.value ?? null;
}

export function trackGoal(event: string, props?: Record<string, unknown>): void {
  const lens = (window as unknown as { lens?: LensGlobal }).lens;
  lens?.track?.(event, props);
  // A conversion shouldn't sit in the 5s batch - push it now so the dashboard reflects
  // it on the next refresh
  lens?.flush?.();
}
