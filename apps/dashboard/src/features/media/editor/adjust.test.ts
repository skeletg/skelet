import { describe, expect, it } from 'vitest'
import { NEUTRAL, applyAdjust, cssFilter, isNeutral } from './adjust'

const px = (...rgba: number[]) => new Uint8ClampedArray(rgba)

describe('light and colour', () => {
  it('leaves a picture alone when nothing is moved', () => {
    const pixels = px(12, 130, 250, 255)
    applyAdjust(pixels, NEUTRAL)
    expect([...pixels]).toEqual([12, 130, 250, 255])
    expect(isNeutral(NEUTRAL)).toBe(true)
    expect(cssFilter(NEUTRAL)).toBe('none')
  })

  it('shows the stage the same three filters it works into the pixels', () => {
    expect(cssFilter({ brightness: 20, contrast: -10, saturation: 50 })).toBe('brightness(1.2) contrast(0.9) saturate(1.5)')
  })

  it('brightens by multiplying, and stops at white', () => {
    const pixels = px(100, 200, 250, 128)
    applyAdjust(pixels, { ...NEUTRAL, brightness: 50 })
    expect([...pixels]).toEqual([150, 255, 255, 128])
  })

  it('pushes contrast away from mid grey, which stays put', () => {
    const pixels = px(128, 28, 228, 255, 127, 127, 127, 255)
    applyAdjust(pixels, { ...NEUTRAL, contrast: 50 })
    expect([...pixels]).toEqual([128, 0, 255, 255, 127, 127, 127, 255])
  })

  it('drains colour to the grey a browser would make of it, and never touches transparency', () => {
    const pixels = px(255, 0, 0, 40, 0, 255, 0, 40)
    applyAdjust(pixels, { ...NEUTRAL, saturation: -100 })
    expect([...pixels]).toEqual([54, 54, 54, 40, 182, 182, 182, 40])
  })

  it('works brightness, then contrast, then saturation, as the filter list does', () => {
    const pixels = px(100, 100, 100, 255)
    applyAdjust(pixels, { brightness: 50, contrast: 50, saturation: -100 })
    // 100 * 1.5 = 150, then (150 - 127.5) * 1.5 + 127.5 = 161.25, grey stays grey
    expect([...pixels]).toEqual([161, 161, 161, 255])
  })
})
