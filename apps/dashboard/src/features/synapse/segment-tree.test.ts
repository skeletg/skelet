import { describe, expect, it } from 'vitest'

import { eventLabel } from './eventLabel'
import { AI_EVENTS, segmentEvents } from './segment-tree'

describe('segmentEvents', () => {
  it('offers the AI events before the store has any', () => {
    expect(segmentEvents(['purchase', 'page_view'])).toEqual(['purchase', 'page_view', 'assistant_turn', 'assisted_order'])
  })

  it('keeps one already seen where the store ranks it', () => {
    expect(segmentEvents(['assistant_turn', 'purchase'])).toEqual(['assistant_turn', 'purchase', 'assisted_order'])
  })

  it('labels them in plain words', () => {
    expect(AI_EVENTS.map(eventLabel)).toEqual(['Chatted with the shopping assistant', 'Ordered with AI help'])
  })
})
