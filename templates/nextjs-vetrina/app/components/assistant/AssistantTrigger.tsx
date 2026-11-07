"use client";

import { useTranslations } from "next-intl";

import { preloadPanel as preload } from "./AssistantDock";
import { PANEL_ID, useAssistant } from "./AssistantProvider";
import { AssistantSpark } from "./AssistantSpark";

export function AssistantTrigger() {
  const { enabled, open, toggle, trigger } = useAssistant();
  const t = useTranslations("assistant");
  if (!enabled) return null;
  return (
    <button
      ref={trigger}
      type="button"
      onClick={toggle}
      onPointerEnter={preload}
      onFocus={preload}
      aria-label={t("open")}
      aria-expanded={open}
      aria-controls={PANEL_ID}
      className={`relative flex h-9 w-9 items-center justify-center rounded-md text-foreground transition hover:bg-muted ${open ? "bg-muted" : ""}`}
    >
      <AssistantSpark className="h-5 w-5" />
    </button>
  );
}
