"use client";

import { useTranslations } from "next-intl";

import { useAssistant } from "./AssistantProvider";
import { AssistantSpark } from "./AssistantSpark";

// Beside Compare on a product page: opens the assistant already asking about this product
export function AskAboutButton({ name }: { name: string }) {
  const { enabled, ask } = useAssistant();
  const t = useTranslations("assistant");
  if (!enabled) return null;
  return (
    <button
      type="button"
      onClick={() => ask(t("starters.product.1", { title: name }))}
      className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-muted"
    >
      <AssistantSpark className="size-4" />
      {t("tellMe")}
    </button>
  );
}
