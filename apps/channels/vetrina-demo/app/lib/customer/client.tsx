"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import { loadHeaderIdentity, type HeaderIdentity } from "@/app/lib/customer/actions-client";

type State = { identity: HeaderIdentity; resolved: boolean }; // resolved distinguishes "not signed in" from "not known yet"

const Ctx = createContext<State>({ identity: null, resolved: false });

export function CustomerProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State>({ identity: null, resolved: false });

  useEffect(() => {
    let live = true;
    loadHeaderIdentity()
      .then((identity) => live && setState({ identity, resolved: true }))
      .catch(() => live && setState({ identity: null, resolved: true }));
    return () => {
      live = false;
    };
  }, []);

  return <Ctx.Provider value={state}>{children}</Ctx.Provider>;
}

export function useCustomerIdentity(): State {
  return useContext(Ctx);
}
