import { optionContentFor, parseVisibleIfGroup, valueContentFor, type ProductOption, type VisibilityRule } from '../productOptionsStaging'
import { conditionText } from './option-draft'
import { INPUT_TYPES } from './option-types'
import { actionText, ruleLabel, type OptionRule } from './rules'

// A row per option in storefront order, a rule in the row after the last answer it reads, and every link between them

export type OptionNodeData = {
  step: number
  title: string
  type: string
  required: boolean
  when: string | null
  choices: { id: string; title: string }[]
  linked: string[]
  more: number
  template: string | null
}
export type RuleNodeData = { title: string; when: string; effects: string[] }
export type StepNodeData = { kind: 'start' | 'cart'; label: string }

export type MapOption = { id: string; row: number; branch: boolean; data: OptionNodeData }
export type MapRule = { id: string; row: number; data: RuleNodeData }
export type LinkKind = 'path' | 'chain' | 'rejoin' | 'show' | 'read' | 'act' | 'fx'
export type Link = { id: string; kind: LinkKind; from: string; to: string; choice?: string | null }
export type OptionsModel = { options: MapOption[]; rules: MapRule[]; rows: number; links: Link[] }

const MAX_CHOICES = 6

function clauseSources(clauses: VisibilityRule[], byId: Map<string, ProductOption>) {
  const out = new Map<string, { optionId: string; choice: string | null }>()
  for (const c of clauses) {
    const parent = byId.get(c.option_id)
    if (!parent) continue
    const picked = c.op === 'anyOf' ? (c.value_ids ?? []).filter((id) => parent.values.some((v) => v.id === id)) : []
    if (picked.length > 0) for (const id of picked) out.set(`${c.option_id}:${id}`, { optionId: c.option_id, choice: id })
    else out.set(c.option_id, { optionId: c.option_id, choice: null })
  }
  return out
}

export function buildOptionsModel(
  options: ProductOption[],
  rules: OptionRule[],
  locale: string,
  defaultLocale: string,
  currency: string,
): OptionsModel {
  const byId = new Map(options.map((o) => [o.id, o]))
  const rowOf = new Map(options.map((o, i) => [o.id, i]))
  const conditionOf = new Map(options.map((o) => [o.id, parseVisibleIfGroup(o.visibleIf)]))
  const referenced = new Set(
    [...options.flatMap((o) => conditionOf.get(o.id)?.rules ?? []), ...rules.flatMap((r) => r.condition.rules ?? [])]
      .filter((c) => c.op === 'anyOf')
      .flatMap((c) => c.value_ids ?? []),
  )

  const mapOptions: MapOption[] = options.map((o, i) => {
    const values = [...o.values].sort((a, b) => a.sortOrder - b.sortOrder)
    const shown = values.filter((v, n) => n < MAX_CHOICES || referenced.has(v.id))
    const condition = conditionOf.get(o.id)
    return {
      id: `opt:${o.id}`,
      row: i,
      branch: !!condition,
      data: {
        step: i + 1,
        title: optionContentFor(o, locale).title || 'Untitled option',
        type: INPUT_TYPES[o.inputType].label,
        required: o.isRequired,
        when: condition ? conditionText(condition, options, locale) ?? 'answers set through the API' : null,
        choices: shown.map((v) => ({ id: v.id, title: valueContentFor(v, locale) || 'Untitled choice' })),
        linked: [],
        more: values.length - shown.length,
        template: o.templateName ?? null,
      },
    }
  })

  const links: Link[] = []
  const firstMain = mapOptions.find((o) => !o.branch)
  links.push({ id: 'path:start', kind: 'path', from: 'start', to: firstMain?.id ?? 'cart' })
  mapOptions.forEach((o, i) => {
    const id = `path:${options[i].id}`
    if (!o.branch) {
      links.push({ id, kind: 'path', from: o.id, to: mapOptions.slice(i + 1).find((x) => !x.branch)?.id ?? 'cart' })
      return
    }
    const next = mapOptions[i + 1]
    links.push({ id, kind: next?.branch ? 'chain' : 'rejoin', from: o.id, to: next?.id ?? 'cart' })
  })

  // A run of options under the same condition draws it once, on the first - the chain carries the rest
  options.forEach((o, i) => {
    const condition = conditionOf.get(o.id)
    const prev = options[i - 1]
    if (!condition || (prev && conditionOf.get(prev.id) && JSON.stringify(prev.visibleIf) === JSON.stringify(o.visibleIf))) return
    for (const [key, s] of clauseSources(condition.rules ?? [], byId)) {
      links.push({ id: `show:${o.id}<-${key}`, kind: 'show', from: `opt:${s.optionId}`, choice: s.choice, to: `opt:${o.id}` })
    }
  })

  const mapRules: MapRule[] = rules.map((r, i) => {
    const id = `rule:${r.id}`
    const reads = clauseSources(r.condition.rules ?? [], byId)
    const last = Math.max(-1, ...[...reads.values()].map((s) => rowOf.get(s.optionId) ?? -1))
    for (const [key, s] of reads) links.push({ id: `${id}<-${key}`, kind: 'read', from: `opt:${s.optionId}`, choice: s.choice, to: id })
    const targets = new Set(r.actions.flatMap((a) => ((a.type === 'REQUIRE' || a.type === 'OPTIONAL') && byId.has(a.optionId) ? [a.optionId] : [])))
    for (const t of targets) links.push({ id: `${id}->${t}`, kind: 'act', from: id, to: `opt:${t}` })
    if (r.actions.some((a) => a.type === 'SET_QUANTITY' || a.type === 'ADJUST_PRICE')) links.push({ id: `${id}->cart`, kind: 'fx', from: id, to: 'cart' })
    return {
      id,
      row: last + 1,
      data: {
        title: ruleLabel(r, locale, defaultLocale) || `Rule ${i + 1}`,
        when: conditionText(r.condition, options, locale) ?? 'never',
        effects: r.actions.map((a) => actionText(a, options, locale, currency)),
      },
    }
  })

  const byNode = new Map(mapOptions.map((o) => [o.id, o]))
  for (const l of links) if (l.choice && !byNode.get(l.from)?.data.linked.includes(l.choice)) byNode.get(l.from)?.data.linked.push(l.choice)
  const rows = Math.max(options.length, ...mapRules.map((r) => r.row + 1))
  return { options: mapOptions, rules: mapRules, rows, links }
}
