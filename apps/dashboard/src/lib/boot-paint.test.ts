import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { BOOT_PAINT_KEY, captureBootPaint, readBootPaint } from './boot-paint'

const setWidth = (w: number) => Object.defineProperty(window, 'innerWidth', { value: w, configurable: true })

describe('boot paint', () => {
  beforeEach(() => {
    localStorage.clear()
    setWidth(1440)
  })
  afterEach(() => {
    document.body.innerHTML = ''
    document.documentElement.classList.remove('dark')
  })

  it('records the shell it finds and reads it back', () => {
    document.body.innerHTML = '<div data-slot="sidebar-wrapper"><div data-slot="sidebar-inner"></div><main data-slot="sidebar-inset"></main></div>'
    document.documentElement.classList.add('dark')
    captureBootPaint()
    const paint = readBootPaint()
    expect(paint?.dark).toBe(true)
    expect(paint?.rail).not.toBeNull()
    expect(paint?.sheet).toMatchObject({ left: 0, top: 0 })
  })

  it('records nothing before the shell has mounted', () => {
    captureBootPaint()
    expect(localStorage.getItem(BOOT_PAINT_KEY)).toBeNull()
  })

  it('stays off on a phone, where the rail is a drawer and the outline would be wrong', () => {
    localStorage.setItem(BOOT_PAINT_KEY, JSON.stringify({ page: 'black', dark: false, rail: null, sheet: { left: 0 } }))
    setWidth(400)
    expect(readBootPaint()).toBeNull()
  })

  it('ignores a corrupt or partial outline rather than painting half a frame', () => {
    localStorage.setItem(BOOT_PAINT_KEY, '{not json')
    expect(readBootPaint()).toBeNull()
    localStorage.setItem(BOOT_PAINT_KEY, JSON.stringify({ page: 'black' }))
    expect(readBootPaint()).toBeNull()
  })
})
