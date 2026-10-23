import { conditionMatches, type Picks } from './optionVisibility'

// The product's option rules, as the server applies them (internal/cart/options_rules.go) - a rule requiring an option
// beats one freeing it, and two rules fixing different quantities is a conflict the add will be refused for
type RuleLike = { condition: unknown; actions: { type: string; optionId?: string | null; quantity?: number | null }[] }

export function optionRuleEffects(rules: readonly RuleLike[], picks: Picks) {
  const require = new Set<string>()
  const optional = new Set<string>()
  let forcedQty: number | null = null
  let conflict = false
  for (const r of rules) {
    if (!conditionMatches(r.condition, picks)) continue
    for (const a of r.actions) {
      if (a.type === 'REQUIRE' && a.optionId) require.add(a.optionId)
      else if (a.type === 'OPTIONAL' && a.optionId) optional.add(a.optionId)
      else if (a.type === 'SET_QUANTITY' && a.quantity != null) {
        if (forcedQty != null && forcedQty !== a.quantity) conflict = true
        forcedQty = a.quantity
      }
    }
  }
  return {
    isRequired: (o: { id: string; isRequired: boolean }) => require.has(o.id) || (o.isRequired && !optional.has(o.id)),
    forcedQty,
    conflict,
  }
}
