import { describe, expect, it } from 'vitest'

// What this pins: an icon-only Button reads as just "button" to a screen reader unless it carries a name - 27 of them
// did. Scans the source, so a new one fails here rather than in someone's screen reader
const sources = import.meta.glob(['/src/**/*.tsx', '!/src/**/*.test.tsx'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>

// Each <Button ...> opening tag, braces balanced so a `>` inside an expression doesn't end it
function openingTags(src: string): { tag: string; body: string; line: number }[] {
  const out: { tag: string; body: string; line: number }[] = []
  for (const m of src.matchAll(/<Button\b/g)) {
    let i = m.index + m[0].length
    let depth = 0
    for (; i < src.length; i++) {
      const c = src[i]
      if (c === '{') depth++
      else if (c === '}') depth--
      else if (c === '>' && depth === 0) break
    }
    const tag = src.slice(m.index, i + 1)
    const close = src.indexOf('</Button>', i)
    out.push({ tag, body: tag.endsWith('/>') || close < 0 ? '' : src.slice(i + 1, close), line: src.slice(0, m.index).split('\n').length })
  }
  return out
}

describe('icon-only buttons', () => {
  it('all carry an accessible name', () => {
    const unnamed = Object.entries(sources).flatMap(([file, src]) =>
      openingTags(src)
        .filter(({ tag, body }) => /size=["{']*["']icon/.test(tag) && !/aria-label|title=/.test(tag) && !body.includes('sr-only'))
        .map(({ line }) => `${file}:${line}`),
    )
    expect(unnamed).toEqual([])
  })
})
