import { describe, expect, it } from 'vitest'
import { defaultCondition, isMoneyCondition, missingMoneyChannel, segmentType } from './rules'

describe('money conditions', () => {
  it('treats spend and the money traits as money, nothing else', () => {
    expect(isMoneyCondition({ kind: 'total_spent_cents' })).toBe(true)
    expect(isMoneyCondition({ kind: 'trait', field: 'monetary_cents' })).toBe(true)
    expect(isMoneyCondition({ kind: 'trait', field: 'predicted_clv_cents' })).toBe(true)
    expect(isMoneyCondition({ kind: 'trait', field: 'rfm_r' })).toBe(false)
    expect(isMoneyCondition({ kind: 'order_count' })).toBe(false)
  })

  it('flags a money condition with no store, since the backend refuses to pool currencies', () => {
    expect(missingMoneyChannel({ conditions: [{ kind: 'total_spent_cents', value: 100 }] })).toBe(true)
    expect(missingMoneyChannel({ conditions: [{ kind: 'total_spent_cents', value: 100, channelId: 'c1' }] })).toBe(false)
    expect(missingMoneyChannel({ conditions: [{ kind: 'order_count', value: 1 }] })).toBe(false)
  })

  it('pre-fills the only store on a new spend condition', () => {
    expect(defaultCondition('total_spent_cents', 'c1').channelId).toBe('c1')
    expect(defaultCondition('total_spent_cents').channelId).toBeUndefined()
  })
})

describe('segment type', () => {
  it('flags rules the backend cannot run ahead of calling them dynamic', () => {
    expect(segmentType({ conditions: [] }, 'total_spent_cents needs a channelId')).toBe('needs-attention')
    expect(segmentType({ conditions: [] }, null)).toBe('dynamic')
    expect(segmentType(null, undefined)).toBe('manual')
  })
})
