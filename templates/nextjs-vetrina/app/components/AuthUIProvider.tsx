"use client";

import { createContext, useCallback, useContext, useState } from "react";

export type AuthView = "login" | "register" | "forgot";

type AuthUI = {
  isOpen: boolean;
  view: AuthView;
  open: (view?: AuthView) => void;
  close: () => void;
  setView: (view: AuthView) => void;
};

const AuthUIContext = createContext<AuthUI | null>(null);

export function AuthUIProvider({ children }: { children: React.ReactNode }) {
  const [isOpen, setIsOpen] = useState(false);
  const [view, setView] = useState<AuthView>("login");
  const open = useCallback((next?: AuthView) => {
    if (next) setView(next);
    setIsOpen(true);
  }, []);
  const close = useCallback(() => setIsOpen(false), []);
  return (
    <AuthUIContext.Provider value={{ isOpen, view, open, close, setView }}>
      {children}
    </AuthUIContext.Provider>
  );
}

export function useAuthUI(): AuthUI {
  const ctx = useContext(AuthUIContext);
  if (!ctx) {
    throw new Error("useAuthUI must be used inside <AuthUIProvider>");
  }
  return ctx;
}
