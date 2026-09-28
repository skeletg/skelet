// Chart colours as CSS variable references, for recharts and SVG props that take a colour string
// rather than a class. The values live in index.css (series-* and the state colours), so charts
// follow the mode without any code here knowing about it

// Categorical: one per series, in order, for anything whose values carry no good/bad meaning
export const SERIES = [
  'var(--color-series-1)',
  'var(--color-series-2)',
  'var(--color-series-3)',
  'var(--color-series-4)',
  'var(--color-series-5)',
  'var(--color-series-6)',
  'var(--color-series-7)',
  'var(--color-series-8)',
] as const

// The "rest" bucket - uncategorised, other, control
export const SERIES_OTHER = 'var(--color-series-other)'

// A single-series chart's colour
export const CHART_ACCENT = SERIES[0]

export const CHART_SUCCESS = 'var(--color-success)'
export const CHART_WARNING = 'var(--color-warning)'
export const CHART_DESTRUCTIVE = 'var(--color-destructive)'
