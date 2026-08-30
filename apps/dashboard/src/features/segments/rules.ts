// Mirror of the backend rule engine's wire shape (internal/segment/rules.go) -
// every kind/field/op here is allowlisted server-side

export type RuleCondition = {
  kind: 'trait' | 'lifecycle' | 'order_count' | 'total_spent_cents' | 'last_order_days' | 'consent' | 'group'
  field?: string
  op?: string
  value?: number
  days?: number
  stage?: string
  channel?: string
  granted?: boolean
  slug?: string
}

export type Rules = {
  logic?: 'and' | 'or'
  conditions: RuleCondition[]
}

export const KIND_LABELS: Record<RuleCondition['kind'], string> = {
  trait: 'Trait',
  lifecycle: 'Lifecycle stage',
  order_count: 'Order count',
  total_spent_cents: 'Total spent',
  last_order_days: 'Last order',
  consent: 'Consent',
  group: 'Customer group',
}

export const TRAIT_FIELDS = [
  { value: 'rfm_r', label: 'RFM recency score' },
  { value: 'rfm_f', label: 'RFM frequency score' },
  { value: 'rfm_m', label: 'RFM monetary score' },
  { value: 'recency_days', label: 'Days since last order' },
  { value: 'frequency', label: 'Order frequency' },
  { value: 'monetary_cents', label: 'Lifetime spend (cents)' },
  { value: 'engagement_score', label: 'Email engagement score' },
  { value: 'churn_risk', label: 'Churn risk (0-100)' },
  { value: 'predicted_clv_cents', label: 'Predicted CLV (cents)' },
  { value: 'next_order_days', label: 'Predicted days to next order' },
]

export const NUMERIC_OPS = [
  { value: 'eq', label: '=' },
  { value: 'neq', label: '≠' },
  { value: 'gt', label: '>' },
  { value: 'gte', label: '≥' },
  { value: 'lt', label: '<' },
  { value: 'lte', label: '≤' },
]

export const LIFECYCLE_STAGES = ['prospect', 'new', 'active', 'at_risk', 'dormant']

export const CONSENT_KINDS = [
  { value: 'marketing_email', label: 'Marketing email' },
  { value: 'marketing_sms', label: 'Marketing SMS' },
]

export function defaultCondition(kind: RuleCondition['kind']): RuleCondition {
  switch (kind) {
    case 'trait':
      return { kind, field: 'rfm_r', op: 'gte', value: 4 }
    case 'lifecycle':
      return { kind, op: 'eq', stage: 'active' }
    case 'order_count':
      return { kind, op: 'gte', value: 1, days: 0 }
    case 'total_spent_cents':
      return { kind, op: 'gte', value: 10000, days: 0 }
    case 'last_order_days':
      return { kind, op: 'lte', value: 30 }
    case 'consent':
      return { kind, channel: 'marketing_email', granted: true }
    case 'group':
      return { kind, slug: '' }
  }
}

const opLabel = (op?: string) => NUMERIC_OPS.find((o) => o.value === op)?.label ?? op ?? ''
const daysLabel = (days?: number) => (days && days > 0 ? ` in last ${days}d` : '')

// One-line human summary - used in save diffs and the list page
export function summarize(c: RuleCondition): string {
  switch (c.kind) {
    case 'trait': {
      const f = TRAIT_FIELDS.find((t) => t.value === c.field)?.label ?? c.field
      return `${f} ${opLabel(c.op)} ${c.value ?? 0}`
    }
    case 'lifecycle':
      return `lifecycle ${c.op === 'neq' ? 'is not' : 'is'} ${c.stage}`
    case 'order_count':
      return `orders ${opLabel(c.op)} ${c.value ?? 0}${daysLabel(c.days)}`
    case 'total_spent_cents':
      return `spent ${opLabel(c.op)} ${((c.value ?? 0) / 100).toFixed(2)}${daysLabel(c.days)}`
    case 'last_order_days':
      return `last order ${opLabel(c.op)} ${c.value ?? 0}d ago`
    case 'consent':
      return `consent ${c.channel} ${c.granted ? 'granted' : 'not granted'}`
    case 'group':
      return `in group "${c.slug}"`
  }
}

export function summarizeRules(rules: Rules | null): string {
  if (!rules || rules.conditions.length === 0) return 'manual membership'
  const joiner = rules.logic === 'or' ? ' OR ' : ' AND '
  return rules.conditions.map(summarize).join(joiner)
}
