// Shared Core Web Vitals helpers - metric keys are the canonical uppercase names the spine stores, and the gauge lives in VitalGauge.tsx

export type VitalMetric = 'LCP' | 'INP' | 'CLS' | 'TTFB'

// Good ≤ / poor > thresholds (Google's CWV) - LCP/INP/TTFB in ms, CLS unitless
export const VITAL_THRESHOLDS: Record<VitalMetric, [number, number]> = {
  LCP: [2500, 4000],
  INP: [200, 500],
  CLS: [0.1, 0.25],
  TTFB: [800, 1800],
}

// Display order: the core trio first, TTFB (diagnostic) last
export const VITAL_ORDER: VitalMetric[] = ['LCP', 'INP', 'CLS', 'TTFB']

export const VITAL_LABEL: Record<VitalMetric, string> = {
  LCP: 'Largest Contentful Paint',
  INP: 'Interaction to Next Paint',
  CLS: 'Cumulative Layout Shift',
  TTFB: 'Time to First Byte',
}

export const VITAL_GREEN = '#0cce6b'
export const VITAL_AMBER = '#ffa400'
export const VITAL_RED = '#ff4e42'

export function isVitalMetric(m: string): m is VitalMetric {
  return m === 'LCP' || m === 'INP' || m === 'CLS' || m === 'TTFB'
}

export function fmtVital(metric: VitalMetric, v: number): string {
  if (metric === 'CLS') return v.toFixed(3)
  if (v >= 1000) return `${(v / 1000).toFixed(2)} s`
  return `${Math.round(v)} ms`
}

export type VitalRating = 'good' | 'ni' | 'poor'

export function vitalRating(metric: VitalMetric, v: number): VitalRating {
  const [good, poor] = VITAL_THRESHOLDS[metric]
  if (v <= good) return 'good'
  if (v <= poor) return 'ni'
  return 'poor'
}

export const RATING_LABEL: Record<VitalRating, string> = {
  good: 'Good',
  ni: 'Needs improvement',
  poor: 'Poor',
}

export function vitalColor(metric: VitalMetric, v: number): string {
  const r = vitalRating(metric, v)
  return r === 'good' ? VITAL_GREEN : r === 'ni' ? VITAL_AMBER : VITAL_RED
}

export function vitalTextClass(metric: VitalMetric, v: number): string {
  const r = vitalRating(metric, v)
  return r === 'good'
    ? 'text-emerald-600 dark:text-emerald-400'
    : r === 'ni'
      ? 'text-amber-600 dark:text-amber-400'
      : 'text-red-600 dark:text-red-400'
}

// Maps a value to a 0..1 arc fill, 1 at the "good" edge and 0 at "poor", linear within the band and clamped beyond - lower is better for every CWV
export function vitalScore(metric: VitalMetric, v: number): number {
  const [good, poor] = VITAL_THRESHOLDS[metric]
  const s = (poor - v) / (poor - good)
  return Math.max(0, Math.min(1, s))
}
