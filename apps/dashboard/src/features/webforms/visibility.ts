// Client-side twin of the Go visibility evaluator in webform/domain.go, for the builder preview and the storefront form
// The server re-evaluates the same logic on submit, so this is only for UX

import type { AnswerValue, ConditionGroup, Field, Rule } from './types'
import { isLayout } from './types'

export function visibleFields(fields: Field[], answers: Record<string, AnswerValue>): Field[] {
  const out: Field[] = []
  let sectionVisible = true
  for (const f of fields) {
    if (f.type === 'section') {
      sectionVisible = evalGroup(f.visibleIf, answers)
      continue
    }
    if (isLayout(f.type)) continue
    if (sectionVisible && evalGroup(f.visibleIf, answers)) out.push(f)
  }
  return out
}

export function isFieldVisible(field: Field, answers: Record<string, AnswerValue>): boolean {
  return evalGroup(field.visibleIf, answers)
}

export function evalGroup(group: ConditionGroup | undefined, answers: Record<string, AnswerValue>): boolean {
  if (!group || ((group.rules?.length ?? 0) === 0 && (group.groups?.length ?? 0) === 0)) {
    return true // no condition → always visible
  }
  const results: boolean[] = [
    ...(group.rules ?? []).map((r) => evalRule(r, answers)),
    ...(group.groups ?? []).map((g) => evalGroup(g, answers)),
  ]
  return group.logic === 'or' ? results.some(Boolean) : results.every(Boolean)
}

function evalRule(rule: Rule, answers: Record<string, AnswerValue>): boolean {
  const got = answers[rule.field]
  switch (rule.op) {
    case 'empty':
      return isEmpty(got)
    case 'notEmpty':
      return !isEmpty(got)
    case 'equals':
      return matches(got, rule.value ?? '', true)
    case 'notEquals':
      return !matches(got, rule.value ?? '', true)
    case 'contains':
      return matches(got, rule.value ?? '', false)
    case 'oneOf':
      return (rule.values ?? []).some((v) => matches(got, v, true))
    case 'gt':
    case 'lt': {
      const a = toNumber(got)
      const b = toNumber(rule.value)
      if (a === undefined || b === undefined) return false
      return rule.op === 'gt' ? a > b : a < b
    }
    default:
      return false
  }
}

function isEmpty(v: AnswerValue): boolean {
  if (v === undefined || v === null) return true
  if (typeof v === 'string') return v.trim() === ''
  if (typeof v === 'boolean') return !v
  if (Array.isArray(v)) return v.length === 0
  return false
}

function matches(v: AnswerValue, target: string, exact: boolean): boolean {
  const cmp = (s: string) => (exact ? s === target : s.includes(target))
  if (typeof v === 'string') return cmp(v)
  if (typeof v === 'boolean') return cmp(String(v))
  if (typeof v === 'number') return cmp(String(v))
  if (Array.isArray(v)) return v.some((s) => cmp(String(s)))
  return false
}

function toNumber(v: AnswerValue): number | undefined {
  if (typeof v === 'number') return v
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v)
    return Number.isFinite(n) ? n : undefined
  }
  return undefined
}
