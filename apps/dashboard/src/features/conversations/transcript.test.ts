import { describe, expect, it } from 'vitest'
import { argChips, firstLine, shownFrom, toolLabel, unanswered, wordsOf } from './transcript'

const A = 'a930ea95-3020-4c0f-bba5-72675f32e576'
const B = 'b6a1dda9-ee6e-4b60-aa50-96b74776cbce'

describe('shownFrom', () => {
  it('turns what the backend read from the recap into chips', () => {
    expect(
      shownFrom([
        { kind: 'PANEL', productId: A, name: 'Canvas Travel Backpack 28L' },
        { kind: 'CARD', productId: B, name: null },
        { kind: 'BUTTON', label: 'Rain jackets', path: '/search' },
        { kind: 'MOVED', path: '/shop/bags' },
        { kind: 'CART' },
        { kind: 'TEXT', text: 'something else entirely' },
      ]),
    ).toEqual([
      { kind: 'panel', name: 'Canvas Travel Backpack 28L', id: A },
      { kind: 'card', name: 'A product', id: B },
      { kind: 'button', label: 'Rain jackets', path: '/search' },
      { kind: 'moved', path: '/shop/bags' },
      { kind: 'cart' },
      { kind: 'text', text: 'something else entirely' },
    ])
    expect(shownFrom([{ kind: 'CARD', productId: null }])).toEqual([])
  })

  it('names every tool, and a new one readably', () => {
    expect(toolLabel('find_similar_products')).toBe('Looked for similar products')
    expect(toolLabel('get_gift_card_balance')).toBe('Get gift card balance')
  })
})

describe('the rest of a turn', () => {
  it("reads a round's words without its suggestions line", () => {
    expect(wordsOf('Here you go.\n[[next: Price? | Does it come in red? ]]')).toBe('Here you go.')
    expect(wordsOf('[[next: a | b]]')).toBe('')
    expect(wordsOf('Mid [[next: a]] and on')).toBe('Mid [[next: a]] and on')
  })

  it('calls a turn unanswered only when the shopper got neither words nor anything shown', () => {
    expect(unanswered({ reply: '', recap: '', incomplete: false })).toBe(true)
    expect(unanswered({ reply: '', recap: 'Shown as cards: Mug (id x)', incomplete: false })).toBe(false)
    expect(unanswered({ reply: 'Hi', recap: '', incomplete: false })).toBe(false)
    expect(unanswered({ reply: '', recap: '', incomplete: true })).toBe(false)
  })

  it('sums up a call in a few chips', () => {
    expect(argChips('{"query":"rain jacket","limit":10,"filters":{"color":["navy","red"]},"ids":["a","b","c","d"],"sort":""}')).toEqual([
      'query: "rain jacket"',
      'limit: 10',
      'filters: color=navy/red',
      'ids: 4 items',
    ])
    expect(argChips(`{"keep_in_mind":["${'x'.repeat(80)}"]}`)[0]).toHaveLength(60)
    expect(argChips('not json')).toEqual([])
    expect(firstLine('1 products match (showing 1-1):\n- Backpack')).toBe('1 products match (showing 1-1):')
  })
})
