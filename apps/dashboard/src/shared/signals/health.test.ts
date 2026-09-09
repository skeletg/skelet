import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setSignalsDown, signalsIsDown, subscribeSignalsHealth } from './health'

// The banner subscribes to this, and a listener that misses the recovery leaves the warning up over screens that are working again, which is worse than never showing it

describe('signals health', () => {
  beforeEach(() => setSignalsDown(false))

  it('starts up and reports what it was last told', () => {
    expect(signalsIsDown()).toBe(false)
    setSignalsDown(true)
    expect(signalsIsDown()).toBe(true)
  })

  it('notifies subscribers on a change, in both directions', () => {
    const seen: boolean[] = []
    const stop = subscribeSignalsHealth((down) => seen.push(down))

    setSignalsDown(true)
    setSignalsDown(false)

    expect(seen).toEqual([true, false])
    stop()
  })

  it('stays quiet when nothing changed, since every request reports success', () => {
    const fn = vi.fn()
    const stop = subscribeSignalsHealth(fn)

    setSignalsDown(false) // already false
    setSignalsDown(true)
    setSignalsDown(true) // still true

    expect(fn).toHaveBeenCalledTimes(1)
    stop()
  })

  it('stops notifying once unsubscribed', () => {
    const fn = vi.fn()
    subscribeSignalsHealth(fn)()
    setSignalsDown(true)
    expect(fn).not.toHaveBeenCalled()
  })
})
