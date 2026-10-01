import { describe, expect, it } from 'vitest'
import { richTextEntry } from './rich-text-diff'

const doc = (text: string, format = 0) => ({
  root: { type: 'root', children: [{ type: 'paragraph', children: [{ type: 'text', text, format }] }] },
})

describe('richTextEntry', () => {
  it('returns null when nothing changed', () => {
    expect(richTextEntry('Description', doc('Hello'), doc('Hello'))).toBeNull()
    expect(richTextEntry('Description', null, undefined)).toBeNull()
  })

  it('shows the plain text when the words changed', () => {
    expect(richTextEntry('Description', doc('Hello'), doc('Goodbye'))).toMatchObject({
      label: 'Description',
      before: 'Hello',
      after: 'Goodbye',
    })
  })

  it('says formatting changed when only the markup did', () => {
    expect(richTextEntry('Description', doc('Hello', 0), doc('Hello', 1))).toMatchObject({
      before: 'previous formatting',
      after: 'updated formatting',
    })
  })

  it('clips long text the same way the rest of the save dialog does', () => {
    const entry = richTextEntry('Description', doc('a'), doc('b'.repeat(300)))
    expect(entry?.after).toHaveLength(120)
  })
})
