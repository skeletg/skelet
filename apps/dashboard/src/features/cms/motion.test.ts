import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { holdInPlace, panelDuration } from './motion'

// What these pin: a low header eases up to a quarter of the scroller, a header that will lose more height above
// it than there is scroll left gives up the difference from the start, a second hold takes over from the first,
// and the admin's own wheel stops the steering

let frames: FrameRequestCallback[] = []
let clock = 0

function runFrame() {
  const pending = frames
  frames = []
  for (const cb of pending) cb(clock)
}

// A 1000px-tall scroller at the top of the window, holding a card whose top moves with the scroll position
function rig(cardTop: number, scrollTop = 0) {
  const scroller = document.createElement('div')
  scroller.style.overflowY = 'auto'
  let top = scrollTop
  Object.defineProperty(scroller, 'scrollTop', { get: () => top, set: (v: number) => (top = v), configurable: true })
  scroller.getBoundingClientRect = () => ({ top: 0, height: 1000 }) as DOMRect
  const card = document.createElement('div')
  card.getBoundingClientRect = () => ({ top: cardTop + scrollTop - top }) as DOMRect
  scroller.appendChild(card)
  document.body.appendChild(scroller)
  return { scroller, card, cardTop: () => card.getBoundingClientRect().top }
}

beforeEach(() => {
  frames = []
  clock = 0
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => frames.push(cb))
  vi.spyOn(performance, 'now').mockImplementation(() => clock)
})

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

describe('holdInPlace', () => {
  it('eases a low header up to a quarter of the scroller', () => {
    const { card, cardTop } = rig(800)
    holdInPlace(card)
    runFrame()
    clock = 1000
    runFrame()
    expect(cardTop()).toBe(250)
  })

  it('gives up whatever the page cannot scroll back for', () => {
    const { card, cardTop } = rig(300, 200)
    holdInPlace(card, { shrinkAbove: 500 })
    runFrame()
    clock = 1000
    runFrame()
    expect(cardTop()).toBe(0)
  })

  it('hands over to a newer hold instead of fighting it', () => {
    const a = rig(800)
    const b = rig(700)
    holdInPlace(a.card)
    holdInPlace(b.card)
    runFrame()
    expect(frames).toHaveLength(1)
  })

  it('stops steering once the admin scrolls', () => {
    const { scroller, card, cardTop } = rig(800)
    holdInPlace(card)
    scroller.dispatchEvent(new Event('wheel'))
    runFrame()
    expect(cardTop()).toBe(800)
    expect(frames).toHaveLength(0)
  })
})

describe('panelDuration', () => {
  it('scales with the distance, within bounds', () => {
    expect(panelDuration(0)).toBe(200)
    expect(panelDuration(500)).toBe(250)
    expect(panelDuration(5000)).toBe(340)
  })
})
