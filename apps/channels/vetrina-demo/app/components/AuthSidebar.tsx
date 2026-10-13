"use client";

import { useEffect, useState } from "react";
import dynamic from "next/dynamic";
import { useTranslations } from "next-intl";

import { Drawer } from "./Drawer";
import { useAuthUI, type AuthView } from "./AuthUIProvider";

// The forms carry zod and Conform, the heaviest thing in the storefront's client bundle - the drawer is
// mounted on every page but opened on few, so they load on first open (or on hover of the trigger, see
// preloadAuthForms) instead of with the page. The /account/login etc. pages import them statically
const loadLogin = () => import("@/app/components/auth/LoginForm").then((m) => m.LoginForm);
const loadRegister = () => import("@/app/components/auth/RegisterForm").then((m) => m.RegisterForm);
const loadForgot = () => import("@/app/components/auth/ForgotForm").then((m) => m.ForgotForm);
const formLoading = () => <div className="h-64" aria-busy="true" />;
const LoginForm = dynamic(loadLogin, { loading: formLoading });
const RegisterForm = dynamic(loadRegister, { loading: formLoading });
const ForgotForm = dynamic(loadForgot, { loading: formLoading });

export function preloadAuthForms(): void {
  void loadLogin();
  void loadRegister();
  void loadForgot();
}

// auth.drawer message keys per view
const TITLES = {
  login: "loginTitle",
  register: "registerTitle",
  forgot: "forgotTitle",
} as const satisfies Record<AuthView, string>;

const SUBTITLES = {
  login: "loginSubtitle",
  register: "registerSubtitle",
  forgot: "forgotSubtitle",
} as const satisfies Record<AuthView, string>;

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
  const t = useTranslations("auth.drawer");
  const tc = useTranslations("common");
  // Read at open time from window rather than useSearchParams - the drawer is mounted in
  // the root layout, and useSearchParams there would need a Suspense boundary
  // Captured on open rather than during render: window is server-undefined, and the value
  // has to be the location as it was when the drawer opened
  const [returnTo, setReturnTo] = useState<string | null>(null);
  // Nothing inside renders until the first open, which is what keeps the forms off the page load -
  // then it stays mounted, so the close animation and a half-typed form both survive
  const [opened, setOpened] = useState(false);
  useEffect(() => {
    if (!isOpen) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setReturnTo(currentReturnTo());
    setOpened(true);
  }, [isOpen]);

  return (
    <Drawer
      open={isOpen}
      onOpenChange={(next) => (next ? null : close())}
      side="right"
      title={t(TITLES[view])}
      hideTitle
    >
      {opened ? (
        <>
          <header className="flex flex-col gap-1 border-b border-border px-5 py-4">
            <h2 className="text-lg font-semibold tracking-tight">
              {t(TITLES[view])}
            </h2>
            <p className="text-xs text-muted-foreground">{t(SUBTITLES[view])}</p>
            <div
              role="tablist"
              aria-label={t("tabs")}
              className="mt-3 inline-flex rounded-md border border-border bg-background p-0.5 text-sm"
            >
              <TabButton
                active={view === "login"}
                onClick={() => setView("login")}
              >
                {tc("signIn")}
              </TabButton>
              <TabButton
                active={view === "register"}
                onClick={() => setView("register")}
              >
                {tc("register")}
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
        </>
      ) : null}
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
