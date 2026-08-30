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
    }).format(amount / 100)
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
