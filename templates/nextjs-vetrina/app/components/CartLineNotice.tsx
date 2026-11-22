import { useTranslations } from "next-intl";

import { formatPrice, type CurrencyInfo } from "@skelet/storefront-core/format";
import type { CartLine } from "@skelet/storefront-core/cart/types";

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
  // The store set this line's price for the shopper - said plainly, with what it usually costs and how many it is for
  const yourPrice = line.listUnitPrice != null && line.listUnitPrice !== line.unitPrice;
  const priceFor = line.priceMaxQuantity ?? null;
  // Said so a shopper whose code skipped the line knows why
  const noPromotions = line.promotionsApply === false;
  if (!stockShort && !priceChanged && !yourPrice && priceFor === null && !noPromotions) return null;

  return (
    <div className="mt-1 space-y-0.5">
      {stockShort && (
        <p className="text-xs font-medium text-amber-700">
          {line.availableQuantity === 0
            ? t("outOfStock")
            : t("onlyLeft", { count: line.availableQuantity ?? 0 })}
        </p>
      )}
      {(yourPrice || priceFor !== null) && (
        <p className="text-xs text-muted-foreground">
          {yourPrice ? t("yourPrice", { regular: formatPrice(line.listUnitPrice as number, currency) }) : t("yourPriceOnly")}
          {priceFor !== null ? ` ${t("priceFor", { count: priceFor })}` : ""}
        </p>
      )}
      {noPromotions && <p className="text-xs text-muted-foreground">{t("noPromotions")}</p>}
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
