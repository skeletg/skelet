import { useEffect } from 'react'

// SYNC: apps/dashboard/index.html paints this outline before any script or stylesheet loads, instead of a white page
export const BOOT_PAINT_KEY = 'skelet-dash:boot-paint'
export const BOOT_PAINT_MIN_WIDTH = 768

type Box = { left: number; top: number; right: number; bottom: number; radius: string; color: string; shadow: string }
export type BootPaint = { page: string; dark: boolean; rail: Box | null; sheet: Box }

function measure(el: Element | null): Box | null {
  if (!el) return null
  const r = el.getBoundingClientRect()
  const cs = getComputedStyle(el)
  return {
    left: Math.round(r.left),
    top: Math.round(r.top),
    right: Math.round(window.innerWidth - r.right),
    bottom: Math.round(window.innerHeight - r.bottom),
    radius: cs.borderRadius,
    color: cs.backgroundColor,
    shadow: cs.boxShadow,
  }
}

function opaque(color: string | undefined): color is string {
  return !!color && color !== 'transparent' && !/rgba\(.*,\s*0\)$/.test(color)
}

export function captureBootPaint() {
  if (window.innerWidth < BOOT_PAINT_MIN_WIDTH) return
  const sheet = measure(document.querySelector('[data-slot="sidebar-inset"]'))
  if (!sheet) return
  const wrapper = document.querySelector('[data-slot="sidebar-wrapper"]')
  const page = [wrapper, document.body, document.documentElement]
    .map((el) => (el ? getComputedStyle(el).backgroundColor : undefined))
    .find(opaque)
  const paint: BootPaint = {
    page: page ?? sheet.color,
    dark: document.documentElement.classList.contains('dark'),
    rail: measure(document.querySelector('[data-slot="sidebar-inner"]')),
    sheet,
  }
  try {
    localStorage.setItem(BOOT_PAINT_KEY, JSON.stringify(paint))
  } catch {
    /* best effort */
  }
}

export function readBootPaint(): BootPaint | null {
  if (typeof window === 'undefined' || window.innerWidth < BOOT_PAINT_MIN_WIDTH) return null
  try {
    const paint = JSON.parse(localStorage.getItem(BOOT_PAINT_KEY) ?? 'null') as BootPaint | null
    return paint && typeof paint.page === 'string' && paint.sheet ? paint : null
  } catch {
    return null
  }
}

const SETTLE_MS = 1500

export function useBootPaintCapture() {
  useEffect(() => {
    const timer = setTimeout(captureBootPaint, SETTLE_MS)
    const onHide = () => {
      if (document.visibilityState === 'hidden') captureBootPaint()
    }
    document.addEventListener('visibilitychange', onHide)
    window.addEventListener('pagehide', captureBootPaint)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', onHide)
      window.removeEventListener('pagehide', captureBootPaint)
    }
  }, [])
}
