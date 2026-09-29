import { formatPrice, roundHalfAwayFromZero } from "@/app/lib/format";
import type {
  ProductCustomOption,
  ProductCustomOptionValue,
} from "@/app/lib/catalog/read";

const SELECT_TYPES = ["dropdown", "radio", "checkbox", "multiselect"];
const SINGLE_SELECT_TYPES = ["dropdown", "radio"];

export function isSelect(t: string) {
  return SELECT_TYPES.includes(t);
}
export function isSingleSelect(t: string) {
  return SINGLE_SELECT_TYPES.includes(t);
}

export function textInputType(t: string): string {
  switch (t) {
    case "date":
      return "date";
    case "date_time":
      return "datetime-local";
    case "time":
      return "time";
    default:
      return "text"; // field
  }
}

export function priceLabel(
  priceValue: number | null,
  priceType: string,
  currency: string,
): string | null {
  if (priceValue == null || priceValue === 0) return null;
  if (priceType === "percent") return `+${priceValue}%`;
  return `+${formatPrice(priceValue, currency)}`;
}

// Per-unit cents one selected value contributes — mirrors the backend's valueContribution so the live total previews
// what the server will charge (source-product price + delta; else the value's fixed/percent adjustment)
export function valueContribution(
  basePrice: number,
  v: ProductCustomOptionValue,
): number {
  if (v.sourceProduct) return v.sourceProduct.defaultPrice + v.priceValue;
  if (v.priceType === "percent")
    return roundHalfAwayFromZero((basePrice * v.priceValue) / 100);
  return v.priceValue;
}

export function optionContribution(basePrice: number, o: ProductCustomOption): number {
  if (o.priceValue == null) return 0;
  if (o.priceType === "percent")
    return roundHalfAwayFromZero((basePrice * o.priceValue) / 100);
  return o.priceValue;
}
