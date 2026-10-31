import type { BundleOption, ProductDetail } from "./read";

export function isSingleSelect(t: BundleOption["inputType"]) {
  return t === "RADIO" || t === "DROPDOWN";
}

// The add-to-cart bounds: a required option needs one even with no explicit minimum, single-select takes at most one
export function pickBounds(o: BundleOption): { min: number; max: number | null } {
  if (isSingleSelect(o.inputType)) return { min: o.isRequired ? 1 : 0, max: 1 };
  return { min: Math.max(o.minSelections, o.isRequired ? 1 : 0), max: o.maxSelections ?? null };
}

// The cart's rule, so the headline is what the line will charge: fixed is the bundle's own price, the percent comes off the whole sum rounded half up
export function bundlePrice(
  p: Pick<ProductDetail, "bundlePricing" | "bundleDiscountPercent" | "price">,
  sum: number,
): number {
  if (p.bundlePricing === "FIXED") return p.price ?? sum;
  if (p.bundlePricing === "SUM_MINUS_PERCENT") return sum - Math.floor((sum * p.bundleDiscountPercent + 50) / 100);
  return sum;
}
