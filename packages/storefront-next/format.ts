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

// Minor-unit digits: the store's configured decimals when the caller has them, else ISO 4217's for the code (JPY 0,
// BHD 3) - never a blanket 2, which shows ¥1,000 as ¥10 and 1.000 BHD as 10.00
export function currencyDecimals(currency: string | CurrencyInfo): number {
  if (typeof currency !== "string" && currency.decimals != null) return currency.decimals;
  const code = (typeof currency === "string" ? currency : currency.code) || "USD";
  try {
    return new Intl.NumberFormat("en", { style: "currency", currency: code }).resolvedOptions().maximumFractionDigits ?? 2;
  } catch {
    return 2;
  }
}

// Minor units to the major-unit number analytics and structured data expect
export function toMajor(amountMinor: number, currency: string | CurrencyInfo): number {
  return amountMinor / Math.pow(10, currencyDecimals(currency));
}

// The store's configured currency when it is the one a price is in, else the bare code (ISO precision)
export function priceCurrency(code: string, configured: CurrencyInfo | null | undefined): string | CurrencyInfo {
  return configured && configured.code === code ? configured : code;
}

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
      }).format(toMajor(amountMinor, code));
    } catch {
      return `${toMajor(amountMinor, code).toFixed(2)} ${code}`;
    }
  }
  const code = currency.code || "USD";
  const decimals = currencyDecimals(currency);
  const symbol = currency.symbol || code;
  const position = currency.symbolPosition === "suffix" ? "suffix" : "prefix";
  const major = toMajor(amountMinor, currency);
  let amount: string;
  try {
    amount = new Intl.NumberFormat("en", {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    }).format(major);
  } catch {
    amount = major.toFixed(decimals);
  }
  return position === "suffix" ? `${amount} ${symbol}` : `${symbol}${amount}`;
}
