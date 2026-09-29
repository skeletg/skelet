"use client";

import { CheckIcon, PlusIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { COMPARE_MAX, toggleCompare, useCompareIds } from "@/app/lib/compare/store";

export function CompareButton({ productId }: { productId: string }) {
  const ids = useCompareIds();
  const t = useTranslations("compare");
  const inList = ids.includes(productId);
  // A full list can still be toggled off, just not added to
  const full = !inList && ids.length >= COMPARE_MAX;

  return (
    <button
      type="button"
      onClick={() => toggleCompare(productId)}
      disabled={full}
      title={full ? t("full", { max: COMPARE_MAX }) : undefined}
      aria-pressed={inList}
      className={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm font-medium transition-colors ${
        inList
          ? "border-foreground bg-foreground text-background"
          : "border-border text-foreground hover:bg-muted disabled:cursor-not-allowed disabled:text-muted-foreground disabled:hover:bg-transparent"
      }`}
    >
      {inList ? (
        <CheckIcon className="size-4" aria-hidden="true" />
      ) : (
        <PlusIcon className="size-4" aria-hidden="true" />
      )}
      {inList ? t("comparing") : t("title")}
    </button>
  );
}
