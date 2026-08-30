"use client";

import { useSyncExternalStore } from "react";

const KEY = "storefront_compare";
const EVENT = "storefront:compare-changed";
export const COMPARE_MAX = 4;

const EMPTY: string[] = [];
// getSnapshot must return a stable reference between changes or React re-renders
// forever - cache the parsed array against the raw string
let cachedRaw: string | null = null;
let cachedIds: string[] = EMPTY;

export function readCompareIds(): string[] {
  let raw: string | null;
  try {
    raw = localStorage.getItem(KEY);
  } catch {
    return EMPTY; // private mode - comparison silently off
  }
  if (raw === cachedRaw) return cachedIds;
  cachedRaw = raw;
  try {
    const list = raw ? (JSON.parse(raw) as unknown) : [];
    cachedIds = Array.isArray(list)
      ? list.filter((v): v is string => typeof v === "string").slice(0, COMPARE_MAX)
      : EMPTY;
  } catch {
    cachedIds = EMPTY;
  }
  return cachedIds;
}

function write(ids: string[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(ids.slice(0, COMPARE_MAX)));
  } catch {
    return;
  }
  window.dispatchEvent(new Event(EVENT));
}

export function toggleCompare(productId: string) {
  const ids = readCompareIds();
  write(
    ids.includes(productId)
      ? ids.filter((id) => id !== productId)
      : [...ids, productId],
  );
}

export function removeFromCompare(productId: string) {
  write(readCompareIds().filter((id) => id !== productId));
}

export function clearCompare() {
  write([]);
}

function subscribe(onChange: () => void) {
  window.addEventListener(EVENT, onChange);
  window.addEventListener("storage", onChange);
  return () => {
    window.removeEventListener(EVENT, onChange);
    window.removeEventListener("storage", onChange);
  };
}

// Server snapshot is always empty - the list is browser-only, so the first client
// render matches the SSR markup and hydration stays quiet
export function useCompareIds(): string[] {
  return useSyncExternalStore(subscribe, readCompareIds, () => EMPTY);
}
