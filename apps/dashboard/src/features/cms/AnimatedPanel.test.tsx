import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AnimatedPanel } from './AnimatedPanel'

// What these pin: an opening panel's clock starts at real time (the frame clock lags after a long mount) and holds
// its first keyframe until then, a closing panel stays until its animation finishes, and reopening mid-close keeps
// it rather than letting the stale finish unmount it

type FakeAnimation = { options: KeyframeAnimationOptions; startTime: number | null; onfinish: (() => void) | null; cancel: () => void }
let animations: FakeAnimation[] = []
const proto = Element.prototype as unknown as Record<string, unknown>

beforeEach(() => {
  animations = []
  proto.animate = function (_keyframes: Keyframe[], options: KeyframeAnimationOptions) {
    const a: FakeAnimation = { options, startTime: null, onfinish: null, cancel: () => {} }
    animations.push(a)
    return a
  }
  proto.getAnimations = () => []
  vi.spyOn(performance, 'now').mockReturnValue(1234)
})

afterEach(() => {
  delete proto.animate
  delete proto.getAnimations
  vi.restoreAllMocks()
})

const panel = (open: boolean) => (
  <AnimatedPanel open={open}>
    <p>Block form</p>
  </AnimatedPanel>
)

describe('AnimatedPanel', () => {
  it('starts the open on real time and holds the first keyframe until then', () => {
    const { rerender } = render(panel(false))
    expect(screen.queryByText('Block form')).toBeNull()
    rerender(panel(true))
    expect(screen.getByText('Block form')).toBeDefined()
    expect(animations[0].startTime).toBe(1234)
    expect(animations[0].options.fill).toBe('backwards')
  })

  it('keeps a closing panel until its animation finishes', () => {
    const { rerender } = render(panel(true))
    rerender(panel(false))
    expect(screen.getByText('Block form')).toBeDefined()
    act(() => animations.at(-1)?.onfinish?.())
    expect(screen.queryByText('Block form')).toBeNull()
  })

  // The same element, not one remounted after the stale finish briefly unmounted it - a remount drops what's typed
  it('stays open when reopened before the close finished', () => {
    const { rerender } = render(panel(true))
    const form = screen.getByText('Block form')
    rerender(panel(false))
    const closing = animations.at(-1)
    rerender(panel(true))
    act(() => closing?.onfinish?.())
    expect(screen.getByText('Block form')).toBe(form)
  })
})
