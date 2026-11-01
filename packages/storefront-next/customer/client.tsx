"use client";

import { usePathname } from "next/navigation";
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import { loadHeaderIdentity, type HeaderIdentity } from "./actions-client";

type State = { identity: HeaderIdentity; resolved: boolean }; // resolved distinguishes "not signed in" from "not known yet"

const Ctx = createContext<State>({ identity: null, resolved: false });

const FRESH_MS = 30_000;

export function CustomerProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [state, setState] = useState<State>({ identity: null, resolved: false });

  useEffect(() => {
    let live = true;
    let latest = 0;
    let inFlight = false;
    let freshUntil = 0;
    const refresh = () => {
      const request = ++latest;
      inFlight = true;
      void loadHeaderIdentity()
        .then((identity) => {
          if (!live || request !== latest) return;
          setState({ identity, resolved: true });
          freshUntil = Date.now() + FRESH_MS;
        })
        .catch(() => {
          // A failed read does not mean signed out. Keep the last known identity;
          // navigation or returning to this tab gives the request another chance.
          if (request === latest) freshUntil = 0;
        })
        .finally(() => {
          if (request === latest) inFlight = false;
        });
    };
    const onReturn = () => {
      if (document.visibilityState === "visible" && !inFlight && Date.now() >= freshUntil) refresh();
    };
    refresh();
    window.addEventListener("focus", onReturn);
    document.addEventListener("visibilitychange", onReturn);
    return () => {
      live = false;
      window.removeEventListener("focus", onReturn);
      document.removeEventListener("visibilitychange", onReturn);
    };
  }, [pathname]);

  return <Ctx.Provider value={state}>{children}</Ctx.Provider>;
}

export function useCustomerIdentity(): State {
  return useContext(Ctx);
}
