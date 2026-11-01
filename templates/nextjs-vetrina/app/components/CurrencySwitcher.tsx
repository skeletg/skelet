"use client";

import { useTranslations } from "next-intl";

import { usePresentment } from "./PresentmentProvider";
import { useChannel } from "./ChannelProvider";

export function CurrencySwitcher() {
  const { options, selected, select } = usePresentment();
  const channel = useChannel();
  const t = useTranslations("currency");
  if (options.length === 0) return null;
  const base = channel?.currencyCode ?? "base";
  return (
    <select
      aria-label={t("label")}
      value={selected?.code ?? base}
      onChange={(e) => select(e.target.value === base ? null : e.target.value)}
      className="h-9 rounded-md border border-transparent bg-transparent px-1 font-mono text-xs text-muted-foreground transition hover:border-border focus:border-input focus:outline-none [&>option]:text-foreground"
    >
      <option value={base}>{base}</option>
      {options.map((o) => (
        <option key={o.code} value={o.code}>
          {o.code}
        </option>
      ))}
    </select>
  );
}
