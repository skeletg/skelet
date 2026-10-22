import { describe, expect, it } from 'vitest'

// What this pins: 'use no memo' only counts as the file's first statement - an import added above it silently hands the
// table back to the React Compiler, and the pagination buttons freeze again
const sources = import.meta.glob(['/src/**/*.{ts,tsx}', '!/src/**/*.test.{ts,tsx}'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>

describe("'use no memo'", () => {
  it('comes before any import', () => {
    const late = Object.entries(sources)
      .filter(([, src]) => /^'use no memo'/m.test(src)) // file-level only - a function can opt out inside its body
      .filter(([, src]) => {
        let code = src.trimStart()
        // comments may sit above it
        while (/^(\/\/|\/\*)/.test(code)) code = code.slice(code.startsWith('//') ? code.indexOf('\n') + 1 : code.indexOf('*/') + 2).trimStart()
        return !code.startsWith("'use no memo'")
      })
      .map(([file]) => file)
    expect(late).toEqual([])
  })
})
