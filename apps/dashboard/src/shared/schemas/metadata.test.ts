import { afterEach, describe, expect, it } from 'vitest'
import { __resetDetailFieldsForTests, registerDetailField } from '@/plugins/field-registry'
import { metadataDiff } from './metadata'

afterEach(() => __resetDetailFieldsForTests())

describe('metadataDiff', () => {
  it('names a plugin field’s key by its label on that page only', () => {
    registerDetailField({ id: 'acme:vip', target: 'customer', key: 'vip', label: 'VIP customer', type: 'checkbox' })
    const before = [{ key: 'note', value: 'a' }]
    const after = [{ key: 'note', value: 'b' }, { key: 'vip', value: 'true' }]
    expect(metadataDiff(before, after, 'Metadata', 'customer')).toEqual([
      { kind: 'change', label: 'Metadata · note', before: 'a', after: 'b' },
      { kind: 'add', label: 'VIP customer', before: '', after: 'Yes' },
    ])
    expect(metadataDiff(before, after)[1]).toEqual({ kind: 'add', label: 'Metadata · vip', before: '', after: 'true' })
  })
})
