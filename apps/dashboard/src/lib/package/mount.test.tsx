import { afterEach, describe, expect, it, vi } from 'vitest'

const render = vi.fn()
vi.mock('react-dom/client', () => ({ createRoot: () => ({ render }) }))
vi.mock('@/lib/i18n', async (original) => ({ ...(await original<typeof import('@/lib/i18n')>()), bootLocale: vi.fn() }))

afterEach(() => render.mockClear())

describe('mounting the dashboard', () => {
  // A plugin adds a page under one of core's menu groups as it's imported, which needs the group to exist - loaded
  // ahead of the dashboard it threw, and nothing ever rendered
  it('loads the plugins once the dashboard’s own menu is there, and before the first render', async () => {
    const { mount } = await import('./mount')
    const { extendGroup } = await import('@/nav/registry')
    let rendersBefore = -1
    await mount(document.createElement('div'), {
      plugins: async () => {
        extendGroup('catalog', [{ title: 'Warranties', path: '/acme-warranties-under-test' }])
        rendersBefore = render.mock.calls.length
      },
    })
    expect(rendersBefore).toBe(0)
    expect(render).toHaveBeenCalledOnce()
  })

  it('still starts when a plugin fails to load', async () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { mount } = await import('./mount')
    await mount(document.createElement('div'), {
      plugins: async () => {
        throw new Error('nav registry: extendGroup target "no-such-group" not found')
      },
    })
    expect(render).toHaveBeenCalledOnce()
    expect(quiet).toHaveBeenCalled()
    quiet.mockRestore()
  })

  it('starts with no plugins at all', async () => {
    const { mount } = await import('./mount')
    await mount(document.createElement('div'))
    expect(render).toHaveBeenCalledOnce()
  })
})
