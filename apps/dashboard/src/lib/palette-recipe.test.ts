import { describe, expect, it } from 'vitest'
import { DEFAULT_TUNE, readTune, tunedVars } from './palette-recipe'

describe('tuned palette recipe', () => {
  it('each dial moves the colours it is named for', () => {
    const base = tunedVars(DEFAULT_TUNE, false)
    expect(tunedVars({ ...DEFAULT_TUNE, accentHue: 240 }, false)['--primary']).not.toBe(base['--primary'])
    expect(tunedVars({ ...DEFAULT_TUNE, surfaceTone: 'warm' }, false)['--background']).not.toBe(base['--background'])
    expect(tunedVars({ ...DEFAULT_TUNE, railTone: 'match' }, false)['--sidebar']).not.toBe(base['--sidebar'])
    const dark = tunedVars(DEFAULT_TUNE, true)
    expect(tunedVars({ ...DEFAULT_TUNE, darkBase: 'black' }, true)['--background']).not.toBe(dark['--background'])
  })

  it('a strength only matters once the surfaces are tinted', () => {
    const white = { ...DEFAULT_TUNE, surfaceTone: 'neutral' as const }
    expect(tunedVars({ ...white, surfaceTint: 'strong' }, false)['--background']).toBe(tunedVars(white, false)['--background'])
  })

  // a gold button with white text is unreadable, so the light accents carry dark text
  it('puts dark text on light accents', () => {
    const gold = tunedVars({ ...DEFAULT_TUNE, accentHue: 85 }, false)
    expect(gold['--primary-foreground']).toMatch(/^oklch\(0\.22 /)
    expect(tunedVars(DEFAULT_TUNE, false)['--primary-foreground']).toMatch(/^oklch\(0\.99 /)
  })

  it('reads junk as the default dial and wraps the hue', () => {
    expect(readTune({ vividness: 'loud', accentHue: 400 })).toEqual({ ...DEFAULT_TUNE, accentHue: 40 })
    expect(readTune({ accentHue: 'red' }).accentHue).toBe(DEFAULT_TUNE.accentHue)
  })
})
