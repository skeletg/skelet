// Port of the backend's conditional-option evaluator (internal/cart/options_visibility.go) so the cart console hides
// the same options the server would - filling a hidden one is rejected with PRODUCT_OPTION_HIDDEN

export type OptionVisibilityRule = {
  option_id: string
  op: 'anyOf' | 'noneOf' | 'filled' | 'empty'
  value_ids?: string[]
}

export type OptionVisibilityGroup = {
  logic?: 'and' | 'or'
  rules?: OptionVisibilityRule[]
  groups?: OptionVisibilityGroup[]
}

export function parseVisibleIf(raw: unknown): OptionVisibilityGroup | null {
  if (raw == null || typeof raw !== 'object' || Array.isArray(raw)) return null
  const g = raw as OptionVisibilityGroup
  if (!Array.isArray(g.rules) && !Array.isArray(g.groups)) return null
  return g
}

export type Picks = { filled: Set<string>; values: Map<string, Set<string>> }

// One ordered pass - a rule may only reference options ordered before it, which is what makes this cycle-free
export function visibleOptionIds<T extends { id: string; visibleIf?: unknown }>(
  options: T[],
  pickedValueIds: (optionId: string) => string[],
  isFilled: (optionId: string) => boolean,
): Set<string> {
  return visibleOptionPicks(options, pickedValueIds, isFilled).visible
}

export function visibleOptionPicks<T extends { id: string; visibleIf?: unknown }>(
  options: T[],
  pickedValueIds: (optionId: string) => string[],
  isFilled: (optionId: string) => boolean,
): { visible: Set<string>; picks: Picks } {
  const picks: Picks = { filled: new Set(), values: new Map() }
  const visible = new Set<string>()
  for (const opt of options) {
    const group = parseVisibleIf(opt.visibleIf)
    if (group && !evalGroup(group, picks)) continue
    visible.add(opt.id)
    const vals = pickedValueIds(opt.id)
    if (vals.length > 0) {
      picks.values.set(opt.id, new Set(vals))
      picks.filled.add(opt.id)
    } else if (isFilled(opt.id)) {
      picks.filled.add(opt.id)
    }
  }
  return { visible, picks }
}

export function conditionMatches(raw: unknown, picks: Picks): boolean {
  const group = parseVisibleIf(raw)
  return group ? evalGroup(group, picks) : false
}

function evalGroup(g: OptionVisibilityGroup, picks: Picks): boolean {
  const or = g.logic === 'or'
  for (const r of g.rules ?? []) {
    const ok = evalRule(r, picks)
    if (or && ok) return true
    if (!or && !ok) return false
  }
  for (const sub of g.groups ?? []) {
    const ok = evalGroup(sub, picks)
    if (or && ok) return true
    if (!or && !ok) return false
  }
  return !or
}

function evalRule(r: OptionVisibilityRule, picks: Picks): boolean {
  switch (r.op) {
    case 'filled':
      return picks.filled.has(r.option_id)
    case 'empty':
      return !picks.filled.has(r.option_id)
    case 'anyOf':
    case 'noneOf': {
      const picked = picks.values.get(r.option_id)
      const hit = (r.value_ids ?? []).some((id) => picked?.has(id) ?? false)
      return r.op === 'anyOf' ? hit : !hit
    }
    default:
      return false // unknown op fails closed, same as the server
  }
}
