// Pure formatting helpers

// Math.round pulls -0.5 up to 0 - the backend rounds half away from zero, so option
// price previews must too or negative percent deltas drift a cent
export function roundHalfAwayFromZero(n: number): number {
  return n < 0 ? -Math.round(-n) : Math.round(n);
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("en", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}

// Currency input for formatPrice's explicit form. All fields except `code` are optional
// - when omitted, formatting falls back to Intl's locale-driven defaults (decimals from
// the locale, symbol position from the locale's currency style)
export type CurrencyInfo = {
  code: string;
  symbol?: string | null;
  symbolPosition?: string | null;
  decimals?: number | null;
};

// Two forms: formatPrice(amount, "USD") leaves symbol position to the locale, while formatPrice(amount, {code, symbol, symbolPosition,
// decimals}) honours the backend's per-currency config regardless of locale - unknown values default to prefix
export function formatPrice(
  amountMinor: number,
  currency: string | CurrencyInfo,
): string {
  if (typeof currency === "string") {
    const code = currency || "USD";
    try {
      return new Intl.NumberFormat("en", {
        style: "currency",
        currency: code,
      }).format(amountMinor / 100);
    } catch {
      return `${(amountMinor / 100).toFixed(2)} ${code}`;
    }
  }
  const code = currency.code || "USD";
  const decimals = currency.decimals ?? 2;
  const symbol = currency.symbol || code;
  const position = currency.symbolPosition === "suffix" ? "suffix" : "prefix";
  const divisor = Math.pow(10, decimals);
  let amount: string;
  try {
    amount = new Intl.NumberFormat("en", {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    }).format(amountMinor / divisor);
  } catch {
    amount = (amountMinor / divisor).toFixed(decimals);
  }
  return position === "suffix" ? `${amount} ${symbol}` : `${symbol}${amount}`;
}
