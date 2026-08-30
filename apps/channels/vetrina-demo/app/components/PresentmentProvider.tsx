"use client";

import { createContext, useContext, useSyncExternalStore } from "react";

export type PresentmentCurrency = {
  code: string;
  name: string;
  symbol: string;
  decimals: number;
  symbolPosition: string;
  factorFromBase: number;
};

type PresentmentContextValue = {
  options: PresentmentCurrency[];
  selected: PresentmentCurrency | null;
  select: (code: string | null) => void;
};

const PresentmentContext = createContext<PresentmentContextValue>({
  options: [],
  selected: null,
  select: () => {},
});

export const PRESENTMENT_COOKIE = "storefront_currency";

function readCookieSelection(): string | null {
  const match = document.cookie.match(new RegExp(`(?:^|; )${PRESENTMENT_COOKIE}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : null;
}

// document.cookie fires no events, so writes announce themselves - one shared set of listeners keeps
// every mounted provider in step
const selectionListeners = new Set<() => void>();

function subscribeToSelection(onChange: () => void): () => void {
  selectionListeners.add(onChange);
  return () => selectionListeners.delete(onChange);
}

function emitSelectionChange() {
  for (const listener of selectionListeners) listener();
}

// The selection lives entirely client-side ON PURPOSE: pages render selection-free on the server so
// the shared cache serves every visitor one copy, and the cookie hydrates the choice after mount -
// carrying it on a server prop instead would leave the switcher writing a cookie nothing reads
// back, snapping to base on the refresh it itself triggers
export function PresentmentProvider({
  options,
  children,
}: {
  options: PresentmentCurrency[];
  children: React.ReactNode;
}) {
  // The cookie IS the store - useSyncExternalStore reads it after hydration and the server snapshot
  // stays null, so the markup the shared cache holds matches what the server rendered
  const selectedCode = useSyncExternalStore(subscribeToSelection, readCookieSelection, () => null);

  const selected = options.find((o) => o.code === selectedCode) ?? null;
  const select = (code: string | null) => {
    if (code) {
      document.cookie = `${PRESENTMENT_COOKIE}=${code}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
    } else {
      document.cookie = `${PRESENTMENT_COOKIE}=; path=/; max-age=0`;
    }
    emitSelectionChange();
  };
  return (
    <PresentmentContext.Provider value={{ options, selected, select }}>
      {children}
    </PresentmentContext.Provider>
  );
}

export function usePresentment(): PresentmentContextValue {
  return useContext(PresentmentContext);
}
export function convertMinor(baseMinor: number, p: PresentmentCurrency): number {
  return Math.round(baseMinor * p.factorFromBase);
}
