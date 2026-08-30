import { formatPrice, type CurrencyInfo } from "@/app/lib/format";
import type { CartLine } from "@/app/lib/cart/types";

export function CartLineNotice({
  line,
  currency,
}: {
  line: CartLine;
  currency: string | CurrencyInfo;
}) {
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
            ? "Out of stock - remove to check out."
            : `Only ${line.availableQuantity} left - reduce the quantity to check out.`}
        </p>
      )}
      {priceChanged && (
        <p className="text-xs text-amber-700">
          Price updated: {formatPrice(line.unitPrice, currency)} &rarr;{" "}
          {formatPrice(line.currentUnitPrice as number, currency)}
        </p>
      )}
    </div>
  );
}
