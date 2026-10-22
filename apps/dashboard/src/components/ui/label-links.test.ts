import { describe, expect, it } from 'vitest'

// What this pins: a <Label> sitting right above its control has to point at it - 146 sat unlinked, so a screen
// reader announced the box unnamed and clicking the text focused nothing. A label heading a group (a list, a
// picker, a preview) is left alone: only one whose very next element is a control counts
const sources = import.meta.glob(['/src/**/*.tsx', '!/src/**/*.test.tsx'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>

const CONTROL = /<(Input|Textarea|textarea|Switch|Select|Checkbox|RadioGroup|input)\b/

function unlinked(src: string): number[] {
  const out: number[] = []
  for (const m of src.matchAll(/<Label\b/g)) {
    let i = m.index + m[0].length
    let depth = 0
    for (; i < src.length; i++) {
      const c = src[i]
      if (c === '{') depth++
      else if (c === '}') depth--
      else if (c === '>' && depth === 0) break
    }
    if (src.slice(m.index, i).includes('htmlFor')) continue
    const close = src.indexOf('</Label>', i)
    if (close < 0 || CONTROL.test(src.slice(i, close))) continue // wraps its control, which names it
    if (/^\s*<(Input|Textarea|textarea|Switch|Select)\b/.test(src.slice(close + '</Label>'.length))) {
      out.push(src.slice(0, m.index).split('\n').length)
    }
  }
  return out
}

describe('form labels', () => {
  it('point at the control right after them', () => {
    const found = Object.entries(sources).flatMap(([file, src]) => unlinked(src).map((line) => `${file}:${line}`))
    expect(found).toEqual([])
  })
})
