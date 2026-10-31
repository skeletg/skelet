"use client";

import { usePathname } from "next/navigation";
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import { loadHeaderIdentity, type HeaderIdentity } from "./actions-client";

type State = { identity: HeaderIdentity; resolved: boolean }; // resolved distinguishes "not signed in" from "not known yet"

const Ctx = createContext<State>({ identity: null, resolved: false });

export function CustomerProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [state, setState] = useState<State>({ identity: null, resolved: false });

  useEffect(() => {
    let live = true;
    let latest = 0;
    const refresh = () => {
      const request = ++latest;
      void loadHeaderIdentity()
        .then((identity) => {
          if (live && request === latest) setState({ identity, resolved: true });
        })
        .catch(() => {
          // A failed read does not mean signed out. Keep the last known identity;
          // navigation or returning to this tab gives the request another chance.
        });
    };
    const onVisible = () => {
      if (document.visibilityState === "visible") refresh();
    };
    refresh();
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      live = false;
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [pathname]);

  return <Ctx.Provider value={state}>{children}</Ctx.Provider>;
}

export function useCustomerIdentity(): State {
  return useContext(Ctx);
}
