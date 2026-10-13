import type { EmailBlock, EmailStyles } from '@/features/email/blocks'

// Mirrors internal/flow NodeData - Stored in each @xyflow node's `data`
export type FlowNodeData = {
  label?: string
  // send_email - references a reusable library email
  emailId?: string
  subject?: string
  blocks?: EmailBlock[]
  styles?: EmailStyles
  // send_sms - plain-text body with {{ merge tags }}
  smsBody?: string
  // wait
  delaySeconds?: number
  // split
  condition?: FlowCondition
  // add_segment
  segmentId?: string
  // webhook
  url?: string
}

// FlowCondition mirrors internal/flow.Condition - the test used by split nodes
// and the flow-level enrollment filter
export type FlowCondition = {
  kind: string
  negate?: boolean
  segmentId?: string
  lifecycle?: string[]
  minRecency?: number
  minFrequency?: number
  minMonetary?: number
  minEngagement?: number
  minOpens?: number
  minClicks?: number
  consentKind?: string
  optedIn?: boolean
}

export const CONDITION_KINDS: { value: string; label: string }[] = [
  { value: 'segment', label: 'In a segment' },
  { value: 'lifecycle', label: 'Lifecycle stage' },
  { value: 'rfm', label: 'RFM / engagement score' },
  { value: 'email_activity', label: 'Email engagement' },
  { value: 'consent', label: 'Marketing consent' },
]

export const LIFECYCLE_STAGES = ['prospect', 'new', 'active', 'at_risk', 'lapsed'] as const

export function defaultCondition(kind: string): FlowCondition {
  switch (kind) {
    case 'segment':
      return { kind: 'segment', segmentId: '' }
    case 'lifecycle':
      return { kind: 'lifecycle', lifecycle: ['active'] }
    case 'rfm':
      return { kind: 'rfm', minEngagement: 50 }
    case 'email_activity':
      return { kind: 'email_activity', minOpens: 1 }
    case 'consent':
      return { kind: 'consent', consentKind: 'marketing_email', optedIn: true }
    default:
      return { kind: 'segment', segmentId: '' }
  }
}

// summarizeCondition renders a one-liner for a condition (split node + filter)
export function summarizeCondition(c?: FlowCondition): string {
  if (!c) return '-'
  const not = c.negate ? 'NOT ' : ''
  switch (c.kind) {
    case 'segment':
    case 'in_segment':
      return `${not}in segment`
    case 'lifecycle':
      return `${not}lifecycle: ${(c.lifecycle ?? []).join(', ') || '-'}`
    case 'rfm':
      return `${not}RFM/engagement ≥ thresholds`
    case 'email_activity':
      return `${not}opened ≥ ${c.minOpens ?? 0}, clicked ≥ ${c.minClicks ?? 0}`
    case 'consent':
      return `${not}${c.optedIn ? 'opted in to' : 'opted out of'} ${c.consentKind || 'marketing'}`
    default:
      return c.kind
  }
}

export const NODE_LABELS: Record<string, string> = {
  trigger: 'Trigger',
  send_email: 'Send email',
  send_sms: 'Send SMS',
  wait: 'Wait',
  split: 'Conditional split',
  add_segment: 'Add to segment',
  webhook: 'Webhook',
}

// Node types an operator can add (the trigger is auto-created, one per flow)
export const ADDABLE_NODES = ['send_email', 'send_sms', 'wait', 'split', 'add_segment', 'webhook'] as const

export function defaultNodeData(type: string): FlowNodeData {
  switch (type) {
    case 'send_email':
      return { subject: 'Hello {{customer.firstName}}', blocks: [{ type: 'heading', text: 'Hi {{customer.firstName}}' }], styles: {} }
    case 'send_sms':
      return { smsBody: 'Hi {{customer.firstName}}! ' }
    case 'wait':
      return { delaySeconds: 86400 }
    case 'split':
      return { condition: { kind: 'segment', segmentId: '' } }
    case 'add_segment':
      return { segmentId: '' }
    case 'webhook':
      return { url: '' }
    default:
      return {}
  }
}

export const TRIGGER_LABELS: Record<string, string> = {
  order_placed: 'Order placed',
  order_shipped: 'Order shipped',
  customer_registered: 'Customer registered',
  order_cancelled: 'Order cancelled',
  payment_refunded: 'Payment refunded',
  email_verification: 'Email verification',
  password_reset: 'Password reset',
  signup_confirmation: 'Signup confirmation',
  account_exists: 'Account already exists',
  cart_abandoned: 'Abandoned cart',
  browse_abandoned: 'Browse abandonment',
  email_opened: 'Email opened',
  email_clicked: 'Email clicked',
  segment_entered: 'Entered a segment',
  back_in_stock: 'Back in stock',
  form_submitted: 'Signup form submitted',
}

// TRANSACTIONAL_TRIGGERS mirror the server's: flows on these deliver receipts / auth links and default to the
// transactional designation (which bypasses the marketing-consent gate)
export const TRANSACTIONAL_TRIGGERS = new Set([
  'order_placed',
  'order_shipped',
  'order_cancelled',
  'payment_refunded',
  'email_verification',
  'password_reset',
  'signup_confirmation',
  'account_exists',
])

// nodeSummary is the one-liner shown on the node body in the canvas
export function nodeSummary(type: string, data: FlowNodeData): string {
  switch (type) {
    case 'send_email':
      return data.subject || '(no subject)'
    case 'send_sms':
      return data.smsBody ? data.smsBody.slice(0, 40) : '(empty SMS)'
    case 'wait': {
      const s = data.delaySeconds ?? 0
      if (s % 86400 === 0) return `${s / 86400} day(s)`
      if (s % 3600 === 0) return `${s / 3600} hour(s)`
      return `${Math.round(s / 60)} min`
    }
    case 'split':
      return summarizeCondition(data.condition)
    case 'add_segment':
      return data.segmentId ? 'segment set' : '(pick a segment)'
    case 'webhook':
      return data.url || '(no URL)'
    default:
      return ''
  }
}
