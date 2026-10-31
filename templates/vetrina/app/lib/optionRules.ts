// Client-side port of the backend's option rules (internal/cart/options_rules.go) - a rule whose condition matches the
// visible picks requires or frees an option, fixes the line quantity or moves the price. The server applies the same
// rules on add-to-cart; this only previews them

import { roundHalfAwayFromZero } from "./format";
import { conditionMatches, type Picks } from "./optionVisibility";

export type ProductOptionRuleAction = {
  type: string; // require | optional | set_quantity | adjust_price
  optionId?: string | null;
  quantity?: number | null;
  priceType?: string | null; // fixed | percent
  priceValue?: number | null;
  priceUnavailable: boolean;
};

export type ProductOptionRule = {
  id: string;
  label: string;
  condition: unknown;
  actions: ProductOptionRuleAction[];
};

export type RuleEffects = {
  // A rule requiring the option beats one freeing it; one no rule touches keeps its own setting
  isRequired: (option: { id: string; isRequired: boolean }) => boolean;
  forcedQty: number | null;
  qtyConflict: boolean;
  // One row per fired rule that moves the price, as the cart line will show it
  adjustments: { id: string; label: string; amount: number }[];
  // A fired rule charges an amount this store has none of - the cart will refuse the configuration
  priceUnavailable: boolean;
};

export function evaluateOptionRules(
  rules: ProductOptionRule[],
  picks: Picks,
  basePrice: number,
): RuleEffects {
  const require = new Set<string>();
  const optional = new Set<string>();
  let forcedQty: number | null = null;
  let qtyConflict = false;
  let priceUnavailable = false;
  const adjustments: RuleEffects["adjustments"] = [];
  for (const rule of rules) {
    if (!conditionMatches(rule.condition, picks)) continue;
    let amount = 0;
    let priced = false;
    for (const a of rule.actions) {
      if (a.type === "require" && a.optionId) require.add(a.optionId);
      else if (a.type === "optional" && a.optionId) optional.add(a.optionId);
      else if (a.type === "set_quantity" && a.quantity != null) {
        if (forcedQty != null && forcedQty !== a.quantity) qtyConflict = true;
        forcedQty = a.quantity;
      } else if (a.type === "adjust_price") {
        priced = true;
        if (a.priceUnavailable) priceUnavailable = true;
        const v = a.priceValue ?? 0;
        amount += a.priceType === "percent" ? roundHalfAwayFromZero((basePrice * v) / 100) : v;
      }
    }
    if (priced) adjustments.push({ id: rule.id, label: rule.label, amount });
  }
  return {
    isRequired: (o) => require.has(o.id) || (o.isRequired && !optional.has(o.id)),
    forcedQty,
    qtyConflict,
    adjustments,
    priceUnavailable,
  };
}
