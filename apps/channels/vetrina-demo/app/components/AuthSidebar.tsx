"use client";

import { useEffect, useState } from "react";

import { ForgotForm } from "@/app/[locale]/account/forgot/ForgotForm";
import { LoginForm } from "@/app/[locale]/account/login/LoginForm";
import { RegisterForm } from "@/app/[locale]/account/register/RegisterForm";
import { Drawer } from "./Drawer";
import { useAuthUI, type AuthView } from "./AuthUIProvider";

const TITLES: Record<AuthView, string> = {
  login: "Sign in",
  register: "Create an account",
  forgot: "Reset your password",
};

const SUBTITLES: Record<AuthView, string> = {
  login: "Welcome back. Sign in to continue.",
  register: "It only takes a minute.",
  forgot: "We'll email you a link to set a new password.",
};

// Auth pages are never a sensible landing spot, and /account/reset/<token> would put a
// live reset secret into a form field the drawer renders
const NO_RETURN = /^\/account\/(login|register|forgot|reset)(\/|$)/;

function currentReturnTo(): string | null {
  const { pathname, search } = window.location;
  if (NO_RETURN.test(pathname)) return null;
  // Query included - dropping it sent people back to an unfiltered/unsorted page
  return pathname + search;
}

export function AuthSidebar() {
  const { isOpen, view, close, setView } = useAuthUI();
  // Read at open time from window rather than useSearchParams - the drawer is mounted in
  // the root layout, and useSearchParams there would need a Suspense boundary
  // Captured on open rather than during render: window is server-undefined, and the value
  // has to be the location as it was when the drawer opened
  const [returnTo, setReturnTo] = useState<string | null>(null);
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (isOpen) setReturnTo(currentReturnTo());
  }, [isOpen]);

  return (
    <Drawer
      open={isOpen}
      onOpenChange={(next) => (next ? null : close())}
      side="right"
      title={TITLES[view]}
      hideTitle
    >
      <header className="flex flex-col gap-1 border-b border-border px-5 py-4">
        <h2 className="text-lg font-semibold tracking-tight">
          {TITLES[view]}
        </h2>
        <p className="text-xs text-muted-foreground">{SUBTITLES[view]}</p>
        <div
          role="tablist"
          aria-label="Authentication"
          className="mt-3 inline-flex rounded-md border border-border bg-background p-0.5 text-sm"
        >
          <TabButton
            active={view === "login"}
            onClick={() => setView("login")}
          >
            Sign in
          </TabButton>
          <TabButton
            active={view === "register"}
            onClick={() => setView("register")}
          >
            Register
          </TabButton>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto px-5 py-5">
        {view === "login" ? (
          <LoginForm returnTo={returnTo} onSwitchView={setView} />
        ) : view === "register" ? (
          <RegisterForm returnTo={returnTo} onSwitchView={setView} />
        ) : (
          <ForgotForm onSwitchView={setView} />
        )}
      </div>
    </Drawer>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`rounded-md px-3 py-1 text-xs font-medium transition ${
        active
          ? "bg-foreground text-background"
          : "text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}
