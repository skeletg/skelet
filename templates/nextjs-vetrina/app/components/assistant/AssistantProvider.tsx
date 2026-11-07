"use client";

import { createContext, useContext, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";

import type { Exchange } from "./turns";

export const PANEL_ID = "assistant-panel";

type Asker = (text: string) => void;
type Continuer = (exchange: Exchange) => void;

type AssistantState = {
  enabled: boolean;
  name: string; // what the store calls its assistant, empty for the template's own wording
  open: boolean;
  toggle: () => void;
  close: () => void;
  ask: Asker;
  onAsk: (send: Asker | null) => void;
  continueWith: Continuer;
  onContinue: (take: Continuer | null) => void;
  trigger: RefObject<HTMLButtonElement | null>;
};

const Ctx = createContext<AssistantState>({
  enabled: false,
  name: "",
  open: false,
  toggle: () => {},
  close: () => {},
  ask: () => {},
  onAsk: () => {},
  continueWith: () => {},
  onContinue: () => {},
  trigger: { current: null },
});

// Hands things to the panel, holding the latest until its code has loaded and it's listening
function useRelay<T>() {
  const to = useRef<((value: T) => void) | null>(null);
  const held = useRef<{ value: T } | null>(null);
  return useMemo(
    () => ({
      send: (value: T) => {
        if (to.current) to.current(value);
        else held.current = { value };
      },
      listen: (fn: ((value: T) => void) | null) => {
        to.current = fn;
        if (!fn || !held.current) return;
        // after StrictMode's rehearsal unmount, which would cancel a turn started now and restore the chat over it
        queueMicrotask(() => {
          const waiting = held.current;
          if (!waiting || !to.current) return;
          held.current = null;
          to.current(waiting.value);
        });
      },
    }),
    [],
  );
}

export function AssistantProvider({ enabled, name = "", children }: { enabled: boolean; name?: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const asks = useRelay<string>();
  const exchanges = useRelay<Exchange>();
  const trigger = useRef<HTMLButtonElement | null>(null);
  const value = useMemo<AssistantState>(
    () => ({
      enabled,
      name,
      open: enabled && open,
      toggle: () => setOpen((o) => !o),
      close: () => setOpen(false),
      ask: (text) => {
        if (!enabled) return;
        setOpen(true);
        asks.send(text);
      },
      onAsk: asks.listen,
      continueWith: (exchange) => {
        if (!enabled) return;
        setOpen(true);
        exchanges.send(exchange);
      },
      onContinue: exchanges.listen,
      trigger,
    }),
    [enabled, name, open, asks, exchanges],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAssistant(): AssistantState {
  return useContext(Ctx);
}
