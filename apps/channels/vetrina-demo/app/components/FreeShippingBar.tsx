import { useTranslations } from "next-intl";

import { formatPrice, type CurrencyInfo } from "@/app/lib/format";

export function FreeShippingBar({
  subtotal,
  thresholdCents,
  currency,
  className = "",
}: {
  subtotal: number;
  thresholdCents: number;
  currency: string | CurrencyInfo;
  className?: string;
}) {
  const t = useTranslations("cart");
  if (!thresholdCents || thresholdCents <= 0) return null;

  const remaining = thresholdCents - subtotal;
  const qualified = remaining <= 0;
  const pct = Math.max(0, Math.min(100, Math.round((subtotal / thresholdCents) * 100)));

  return (
    <div className={className}>
      <p className="text-xs text-muted-foreground">
        {qualified ? (
          <span className="font-medium text-emerald-700">
            {t("freeShippingUnlocked")}
          </span>
        ) : (
          t.rich("freeShippingRemaining", {
            amount: () => (
              <span className="font-medium text-foreground">
                {formatPrice(remaining, currency)}
              </span>
            ),
          })
        )}
      </p>
      <div
        className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-border"
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={t("freeShippingProgress")}
      >
        <div
          className={`h-full rounded-full transition-all ${qualified ? "bg-emerald-500" : "bg-signal"}`}
          style={{ width: `${qualified ? 100 : pct}%` }}
        />
      </div>
    </div>
  );
}
