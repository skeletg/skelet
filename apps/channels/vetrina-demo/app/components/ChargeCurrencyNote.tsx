"use client";

import { useChannel } from "./ChannelProvider";
import { usePresentment } from "./PresentmentProvider";

export function ChargeCurrencyNote() {
  const { selected } = usePresentment();
  const channel = useChannel();
  if (!selected) return null;
  return (
    <p className="mt-2 text-xs text-muted-foreground">
      {selected.code} amounts are estimates at today&rsquo;s rate. Your
      order is charged in {channel?.currencyCode ?? "the store currency"}.
    </p>
  );
}
