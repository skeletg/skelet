import { describe, expect, it } from 'vitest'

// What this pins: state colours go through the success / warning / info / destructive tokens, which follow the theme and
// dark mode - a raw hue like text-amber-600 doesn't, and the category page's store warning was the last one
const sources = import.meta.glob(['/src/**/*.{ts,tsx}', '!/src/**/*.test.{ts,tsx}'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>

const RAW_HUE = /\b(?:bg|text|border|ring|fill|stroke|from|to)-(?:red|green|emerald|amber|yellow|orange|blue|sky|rose|lime|teal|cyan|indigo|violet|purple|pink|fuchsia)-\d{2,3}\b/g

describe('theme tokens', () => {
  it('leave no raw hue classes in the dashboard', () => {
    const found = Object.entries(sources).flatMap(([file, src]) => [...src.matchAll(RAW_HUE)].map((m) => `${file}: ${m[0]}`))
    expect(found).toEqual([])
  })
})
