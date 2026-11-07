"use client";

import { Component, lazy, Suspense, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";

import { AssistantOrb } from "./AssistantOrb";
import { useAssistant } from "./AssistantProvider";

let loading: Promise<typeof import("./AssistantPanel")> | null = null;

// A failed chunk is forgotten, so the next try fetches it again instead of replaying the failure
export function loadPanel() {
  loading ??= import("./AssistantPanel").catch((err: unknown) => {
    loading = null;
    throw err;
  });
  return loading;
}

export const preloadPanel = () => void loadPanel().catch(() => {});

const lazyPanel = () => lazy(() => loadPanel().then((m) => ({ default: m.AssistantPanel })));

class PanelBoundary extends Component<{ onRetry: () => void; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  retry = () => {
    this.setState({ failed: false });
    this.props.onRetry();
  };

  render() {
    return this.state.failed ? <PanelNotice onRetry={this.retry} /> : this.props.children;
  }
}

export function AssistantDock({ storeName }: { storeName: string }) {
  const { enabled, open } = useAssistant();
  const [everOpened, setEverOpened] = useState(false);
  // lazy remembers a failure for good, so a retry needs a fresh one
  const [AssistantPanel, setPanel] = useState(lazyPanel);
  if (open && !everOpened) setEverOpened(true);
  if (!enabled || !everOpened) return null;
  return (
    <PanelBoundary onRetry={() => setPanel(() => lazyPanel())}>
      <Suspense fallback={<PanelNotice />}>
        <AssistantPanel storeName={storeName} />
      </Suspense>
    </PanelBoundary>
  );
}

function PanelNotice({ onRetry }: { onRetry?: () => void }) {
  const { open } = useAssistant();
  const t = useTranslations("assistant");
  if (!open) return null;
  return (
    <div
      role={onRetry ? "alert" : "status"}
      className="fixed right-4 bottom-4 z-[60] flex max-w-[calc(100vw-2rem)] items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 text-sm text-card-foreground shadow-lg"
    >
      <AssistantOrb size={18} busy={!onRetry} />
      <span>{onRetry ? t("loadFailed") : t("loading")}</span>
      {onRetry ? (
        <button type="button" onClick={onRetry} className="font-semibold underline underline-offset-2 hover:text-signal">
          {t("retry")}
        </button>
      ) : null}
    </div>
  );
}
