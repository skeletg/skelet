"use client";

import { useTranslations } from "next-intl";

import { useChannel } from "./ChannelProvider";
import { usePresentment } from "./PresentmentProvider";

export function ChargeCurrencyNote() {
  const { selected } = usePresentment();
  const channel = useChannel();
  const t = useTranslations("currency");
  if (!selected) return null;
  return (
    <p className="mt-2 text-xs text-muted-foreground">
      {t("estimateNote", {
        code: selected.code,
        base: channel?.currencyCode ?? t("storeCurrency"),
      })}
    </p>
  );
}
