// Monetary amounts are integer cents (BIGINT) on the backend, formatted here against a
// specific currency - orders carry a per-row currencyCode
// Codes in the runtime registry render with the backend's own symbol/position hints;
// everything else falls back to Intl until CurrencyRegistryProvider's query resolves

type CurrencyHint = {
  symbol: string
  symbolPosition: string  // backend string - anything but "suffix" renders as prefix, matching the channel default
  decimals: number  // JPY and friends break the Intl-only path, whose cents divisor is hardcoded to 100
}

const registry = new Map<string, CurrencyHint>()

export function setCurrencyRegistry(
  currencies: readonly { code: string; symbol: string; symbolPosition: string; decimals: number }[],
): void {
  registry.clear()
  for (const c of currencies) {
    registry.set(c.code, {
      symbol: c.symbol,
      symbolPosition: c.symbolPosition,
      decimals: c.decimals,
    })
  }
}

// Minor-unit digits: the registry's hint, else what Intl knows (JPY 0, BHD 3), else 2
export function currencyDecimals(currencyCode: string): number {
  const hint = registry.get(currencyCode)
  if (hint) return hint.decimals
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency: currencyCode }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

// What an admin types ("12.50") to integer minor units - null when blank, not a number, or more decimals than the currency has,
// so "12.50" in JPY or "1.234" in USD is refused instead of rounded into a different amount
export function toMinor(v: string, currencyCode: string): number | null {
  const t = v.trim()
  if (t === '') return null
  const scaled = Number(t) * 10 ** currencyDecimals(currencyCode)
  if (!Number.isFinite(scaled)) return null
  const minor = Math.round(scaled)
  return Math.abs(minor - scaled) < 1e-6 ? minor : null
}

// Integer minor units back to the plain text an admin would type - "12.50", or "1250" for JPY
export function fromMinor(amount: number, currencyCode: string): string {
  const d = currencyDecimals(currencyCode)
  return (amount / 10 ** d).toFixed(d)
}

export function formatCents(amount: number, currencyCode: string): string {
  const hint = registry.get(currencyCode)
  if (hint && hint.symbol) {
    const decimals = hint.decimals
    const divisor = Math.pow(10, decimals)
    let formatted: string
    try {
      formatted = new Intl.NumberFormat(undefined, {
        minimumFractionDigits: decimals,
        maximumFractionDigits: decimals,
      }).format(amount / divisor)
    } catch {
      formatted = (amount / divisor).toFixed(decimals)
    }
    return hint.symbolPosition === 'suffix'
      ? `${formatted} ${hint.symbol}`
      : `${hint.symbol}${formatted}`
  }
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: currencyCode,
    }).format(amount / 10 ** currencyDecimals(currencyCode))
  } catch {
    // Intl throws RangeError on an empty or malformed code, which takes the whole page down through the error boundary - an aggregate with
    // no currency of its own (a cross-currency total, or a figure read before its report resolves) is worth rendering as a plain number
    const plain = new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(amount / 100)
    return currencyCode ? `${plain} ${currencyCode}` : plain
  }
}

// Dates arrive as ISO strings from the API - missing or unparseable renders as '-', so a
// null column never shows up as 1970 or "Invalid Date"
function toDate(v: unknown): Date | null {
  if (v == null || v === '') return null
  const d = new Date(v as string)
  return Number.isNaN(d.getTime()) ? null : d
}

// "Mar 4, 2026, 3:15 PM"
export function formatDateTime(v: unknown): string {
  return toDate(v)?.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) ?? '-'
}

// "Mar 4, 2026"
export function formatDate(v: unknown): string {
  return toDate(v)?.toLocaleDateString(undefined, { dateStyle: 'medium' }) ?? '-'
}

// "Mar 4" - a chart axis or bucket label, falling back to the raw value so a bucket key that isn't a date still reads
export function formatShortDay(v: string): string {
  return toDate(v)?.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) ?? v
}

// The browser's default date-and-time rendering, seconds included - for run logs, where they matter
export function formatTimestamp(v?: string | null): string {
  if (!v) return '-'
  return new Date(v).toLocaleString()
}

export function formatNumber(n: number): string {
  return n.toLocaleString()
}

// "5m ago" - '' for an unparseable value
export function timeAgo(iso: string): string {
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  const s = Math.max(0, Math.round((Date.now() - t) / 1000))
  if (s < 60) return 'just now'
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.round(h / 24)}d ago`
}
