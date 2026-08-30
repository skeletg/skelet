import { formatPrice, type CurrencyInfo } from "@/app/lib/format";

export function CartCreditHint({
  company,
  subtotal,
  currency,
}: {
  company: { creditLimitCents: number; creditUsedCents: number } | null;
  subtotal: number;
  currency: string | CurrencyInfo;
}) {
  if (!company || company.creditLimitCents <= 0) return null;

  const remaining = company.creditLimitCents - company.creditUsedCents;
  const exceeds = subtotal > remaining;

  return (
    <div
      className={`mt-6 rounded-lg px-4 py-3 text-sm ${
        exceeds ? "border border-amber-300 bg-amber-50" : "glass-soft"
      }`}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground">Available company credit</span>
        <span className="font-medium tabular-nums">
          {formatPrice(Math.max(0, remaining), currency)}
          <span className="text-muted-foreground">
            {" "}of {formatPrice(company.creditLimitCents, currency)}
          </span>
        </span>
      </div>
      {exceeds && (
        <p className="mt-1.5 text-amber-700">
          This order exceeds your remaining credit. Pay on account will be
          declined at checkout - choose another payment method or contact your
          account manager.
        </p>
      )}
    </div>
  );
}
