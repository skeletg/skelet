import { useTranslations } from "next-intl";

import { formatPrice, type CurrencyInfo } from "@/app/lib/format";
import type { CartLine } from "@/app/lib/cart/types";

export function CartLineNotice({
  line,
  currency,
}: {
  line: CartLine;
  currency: string | CurrencyInfo;
}) {
  const t = useTranslations("cart");
  const stockShort =
    line.availableQuantity != null && line.availableQuantity < line.quantity;
  const priceChanged =
    line.currentUnitPrice != null && line.currentUnitPrice !== line.unitPrice;
  if (!stockShort && !priceChanged) return null;

  return (
    <div className="mt-1 space-y-0.5">
      {stockShort && (
        <p className="text-xs font-medium text-amber-700">
          {line.availableQuantity === 0
            ? t("outOfStock")
            : t("onlyLeft", { count: line.availableQuantity ?? 0 })}
        </p>
      )}
      {priceChanged && (
        <p className="text-xs text-amber-700">
          {t("priceUpdated", {
            from: formatPrice(line.unitPrice, currency),
            to: formatPrice(line.currentUnitPrice as number, currency),
          })}
        </p>
      )}
    </div>
  );
}
