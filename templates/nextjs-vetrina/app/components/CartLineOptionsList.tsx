import { useTranslations } from "next-intl";

import { formatPrice, type CurrencyInfo } from "@skelet/storefront-next/format";
import type { CartLineOption } from "@skelet/storefront-next/cart/types";

// Shared "optionTitle: value" summary for a configured cart line - the cart page, the
// drawer, and the checkout review all show the same picks
export function CartLineOptionsList({
  options,
  currency,
  className = "mt-2 space-y-0.5 text-xs text-muted-foreground",
}: {
  options: CartLineOption[];
  // Present = show the price delta next to each pick
  currency?: string | CurrencyInfo;
  className?: string;
}) {
  const t = useTranslations("common");
  if (options.length === 0) return null;
  return (
    <ul className={className}>
      {options.map((o) => (
        <li key={o.id}>
          {/* A rule's price row has a name and an amount, no answer */}
          {o.inputType === "rule" ? (
            <span className="text-foreground">{o.optionTitle}</span>
          ) : (
            <>{o.optionTitle}:{" "}</>
          )}
          {o.inputType === "rule" ? null : o.fileUrl ? (
            <a
              href={o.fileUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground underline underline-offset-2"
            >
              {o.textValue || t("download")}
            </a>
          ) : (
            <span className="text-foreground">
              {o.valueLabel || o.textValue}
            </span>
          )}
          {currency != null &&
            o.priceDelta !== 0 &&
            ` (${o.priceDelta > 0 ? "+" : ""}${formatPrice(o.priceDelta, currency)})`}
        </li>
      ))}
    </ul>
  );
}
