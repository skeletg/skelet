import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useStreamed } from './useAssistant'

const motion = (reduced: boolean) => vi.stubGlobal('matchMedia', (query: string) => ({ matches: reduced, media: query, addEventListener() {}, removeEventListener() {} }))

const stream = (initialProps: { text: string; k: string }) => renderHook(({ text, k }) => useStreamed(text, k), { initialProps })

const step = (words = 1) => {
  for (let i = 0; i < words; i++) act(() => vi.advanceTimersByTime(45))
}

beforeEach(() => vi.useFakeTimers())

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('useStreamed', () => {
  it('writes word by word, and starts over when the key changes', () => {
    motion(false)
    const { result, rerender } = stream({ text: 'one two three', k: 'a' })
    expect(result.current).toEqual({ text: '', streaming: true })
    step()
    expect(result.current.text).toBe('one ')
    step(2)
    expect(result.current).toEqual({ text: 'one two three', streaming: false })

    rerender({ text: 'one two three', k: 'b' })
    expect(result.current).toEqual({ text: '', streaming: true })
    step()
    expect(result.current.text).toBe('one ')
    step(2)
    expect(result.current.streaming).toBe(false)
  })

  it('shows the whole text at once where motion is reduced', () => {
    motion(true)
    const { result, rerender } = stream({ text: 'one two three', k: 'a' })
    expect(result.current).toEqual({ text: 'one two three', streaming: false })
    rerender({ text: 'four five', k: 'b' })
    expect(result.current).toEqual({ text: 'four five', streaming: false })
  })
})
