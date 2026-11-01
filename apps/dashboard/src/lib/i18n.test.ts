import { afterEach, describe, expect, it, vi } from 'vitest'

describe('i18n', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('formats placeholders and plurals in a production build, where lingui ships no compiler of its own', async () => {
    vi.stubEnv('NODE_ENV', 'production')
    vi.resetModules()
    const { i18n } = await import('./i18n')
    expect(i18n._({ id: 'pager-of', message: 'of {lastPage}', values: { lastPage: 4047 } })).toBe('of 4047')
    expect(
      i18n._({ id: 'pager-rows', message: '{totalRows, plural, one {# row} other {# rows}}', values: { totalRows: 1 } }),
    ).toBe('1 row')
  })
})
