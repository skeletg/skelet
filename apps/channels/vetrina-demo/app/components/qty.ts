// Canonical quantity-bounds derivation, shared by every PDP (simple, configurable, bundle, grouped, custom-options) and
// the PLP add form so the stepper behaves identically everywhere: same min/max/step/decimals rules, server-mirrored

export type QtyBoundsInput = {
  salableQuantity?: number | null;
  maxOrderQty?: number | null;
  minOrderQty?: number | null;
  quantityStep?: number | null;
  quantityDecimals?: number | null;
};

export type QtyBounds = {
  min: number | undefined; // undefined = no explicit floor (stepper uses step)
  max: number | undefined; // undefined = no ceiling (stepper caps at 99)
  step: number;
  decimals: number;
  defaultValue: number;
};

// Mirrors the server's order-quantity enforcement for UX - max is the tighter of salable
// stock and maxOrderQty, step falls back to the smallest unit at the configured precision
export function deriveQtyBounds(p: QtyBoundsInput): QtyBounds {
  const caps = [p.salableQuantity, p.maxOrderQty].filter(
    (v): v is number => v != null && v > 0,
  );
  const max = caps.length > 0 ? Math.min(...caps) : undefined;
  const decimals = p.quantityDecimals ?? 0;
  const step =
    p.quantityStep && p.quantityStep > 0
      ? p.quantityStep
      : decimals > 0
        ? Math.pow(10, -decimals)
        : 1;
  const min = p.minOrderQty && p.minOrderQty > 0 ? p.minOrderQty : undefined;
  const defaultValue = min && min > step ? min : step;
  return { min, max, step, decimals, defaultValue };
}

function roundTo(n: number, decimals: number): number {
  if (decimals <= 0) return Math.round(n);
  const f = Math.pow(10, decimals);
  return Math.round(n * f) / f;
}

// Smallest sellable quantity: the minimum rounded up onto the step grid
export function floorQty(b: QtyBounds): number {
  const step = b.step > 0 ? b.step : 1;
  return roundTo(Math.ceil((b.min ?? step) / step) * step, b.decimals);
}

// Snaps n onto the step grid within [floor, max]; 0 stays 0 (not selected)
export function snapQty(n: number, b: QtyBounds): number {
  if (n <= 0) return 0;
  const step = b.step > 0 ? b.step : 1;
  const snapped = roundTo(Math.round(n / step) * step, b.decimals);
  return Math.min(Math.max(snapped, floorQty(b)), b.max ?? Infinity);
}
