"use client";

import { formatPrice } from "@skelet/storefront-core/format";
import { convertMinor, usePresentment } from "./PresentmentProvider";

export function ApproxPrice({
  cents,
  className,
}: {
  cents: number;
  className?: string;
}) {
  const { selected } = usePresentment();
  if (!selected) return null;
  const approx = formatPrice(convertMinor(cents, selected), {
    code: selected.code,
    symbol: selected.symbol,
    symbolPosition: selected.symbolPosition,
    decimals: selected.decimals,
  });
  return (
    <span className={className ?? "text-sm font-normal text-muted-foreground"}>
      {" "}
      ≈ {approx}
    </span>
  );
}
