import { describe, expect, it } from 'vitest'

import { eventLabel } from './eventLabel'
import { AI_EVENTS, segmentEvents } from './segment-tree'

describe('segmentEvents', () => {
  it('offers the AI events before the store has any', () => {
    expect(segmentEvents(['order_placed', 'page_view'])).toEqual(['order_placed', 'page_view', 'assistant_turn', 'assisted_order'])
  })

  it('keeps one already seen where the store ranks it', () => {
    expect(segmentEvents(['assistant_turn', 'order_placed'])).toEqual(['assistant_turn', 'order_placed', 'assisted_order'])
  })

  it('labels them in plain words', () => {
    expect(AI_EVENTS.map(eventLabel)).toEqual(['Chatted with the shopping assistant', 'Ordered with AI help'])
  })
})
