"use client";

// Prism - frontend A/B SDK. Fetches active experiments from /t/experiments, assigns the
// visitor deterministically from the Lens anon id (same FNV-1a bucketing as the Go
// backend, so client + server agree), renders the assigned variation's value, and fires
// a one-shot exposure through the Lens tracker (window.lens.track)

import { useEffect, useState } from "react";
import { ensureAnonId } from "@skelet/storefront-core/analytics/visitor";
import { assign, type Experiment } from "@skelet/storefront-core/experiments/assign";
import { beaconFetch } from "@skelet/storefront-core/beacons/client";
import { BEACON } from "@skelet/storefront-core/beacons/paths";


let cache: Promise<Experiment[]> | null = null;
function loadExperiments(): Promise<Experiment[]> {
  if (!cache) {
    cache = beaconFetch(BEACON.experiments, { headers: { Accept: "application/json" } })
      .then((r) => (r.ok ? r.json() : []))
      .catch(() => [] as Experiment[]);
  }
  return cache;
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

      const anon = ensureAnonId();
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
