import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_MEDIA_FILE_ENDPOINT } from '@/lib/api-endpoint'
import { AssetImage } from './AssetImage'

afterEach(() => vi.unstubAllGlobals())

const boot = { id: 'boot', visibility: 'PUBLIC' as const, mimeType: 'image/jpeg', sizeBytes: 2048, url: 'https://cdn/boot.jpg', thumbnailUrl: 'https://cdn/derived/w640/boot.jpg', altText: 'A boot' }

describe('AssetImage', () => {
  it('shows the small copy, and the original once that fails to load', () => {
    render(<AssetImage asset={boot} className="size-full" />)
    const img = screen.getByAltText('A boot') as HTMLImageElement
    expect(img.src).toBe(boot.thumbnailUrl)
    expect(img.className).toBe('size-full')
    expect(img.getAttribute('loading')).toBe('lazy')

    fireEvent.error(img)
    expect(img.src).toBe(boot.url)
  })

  it('tries the small copy again when the asset brings a new one', () => {
    const { rerender } = render(<AssetImage asset={boot} />)
    fireEvent.error(screen.getByAltText('A boot'))
    rerender(<AssetImage asset={{ ...boot, thumbnailUrl: 'https://cdn/derived/w640/boot-2.jpg' }} />)
    expect((screen.getByAltText('A boot') as HTMLImageElement).src).toBe('https://cdn/derived/w640/boot-2.jpg')
  })

  it('asks the API for a copy that is yet to be made', () => {
    render(<AssetImage asset={{ ...boot, thumbnailUrl: '/api/media/t/w640/shoes/boot.jpg' }} />)
    expect((screen.getByAltText('A boot') as HTMLImageElement).src).toBe(`${window.location.origin}/api/media/t/w640/shoes/boot.jpg`)
  })

  it('shows a private picture from the API, at grid size, and nothing until it arrives', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:kept')
    const fetching = vi.fn(async () => new Response(new Blob(['jpg'])))
    vi.stubGlobal('fetch', fetching)
    const { container } = render(<AssetImage asset={{ ...boot, id: 'hidden-boot', visibility: 'PRIVATE', url: '', thumbnailUrl: '' }} className="size-full" />)
    expect(screen.queryByAltText('A boot')).toBeNull()
    expect(container.querySelector('span.size-full')).not.toBeNull()

    expect(((await screen.findByAltText('A boot')) as HTMLImageElement).src).toBe('blob:kept')
    expect(fetching.mock.calls.map((c) => (c as unknown[])[0])).toEqual([`${ADMIN_MEDIA_FILE_ENDPOINT}/hidden-boot?w=640`])
  })

  it('asks for a small private picture whole when the server cannot resize its kind', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:kept')
    const fetching = vi.fn(async () => new Response(new Blob(['gif'])))
    vi.stubGlobal('fetch', fetching)
    render(<AssetImage asset={{ ...boot, id: 'hidden-reel', visibility: 'PRIVATE', mimeType: 'image/gif', url: '', thumbnailUrl: '' }} />)
    await screen.findByAltText('A boot')
    expect(fetching.mock.calls.map((c) => (c as unknown[])[0])).toEqual([`${ADMIN_MEDIA_FILE_ENDPOINT}/hidden-reel`])
  })

  it('never draws a private SVG, which on a blob address would run as the dashboard', async () => {
    const fetching = vi.fn(async () => new Response(new Blob(['<svg/>'])))
    vi.stubGlobal('fetch', fetching)
    const { container } = render(<AssetImage asset={{ ...boot, id: 'hidden-logo', visibility: 'PRIVATE', mimeType: 'image/svg+xml', url: '', thumbnailUrl: '' }} />)
    await new Promise((r) => setTimeout(r, 20))
    expect(fetching).not.toHaveBeenCalled()
    expect(container.querySelector('svg.lucide-image')).not.toBeNull()
  })

  it('shows an icon, not a blank tile, for a private picture that could not be had or drawn', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:kept')
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 502 })))
    const refused = render(<AssetImage asset={{ ...boot, id: 'hidden-refused', visibility: 'PRIVATE', url: '', thumbnailUrl: '' }} />)
    await waitFor(() => expect(refused.container.querySelector('svg.lucide-image')).not.toBeNull())
    refused.unmount()

    vi.stubGlobal('fetch', vi.fn(async () => new Response(new Blob(['not a picture']))))
    const broken = render(<AssetImage asset={{ ...boot, id: 'hidden-broken', visibility: 'PRIVATE', url: '', thumbnailUrl: '' }} />)
    fireEvent.error(await screen.findByAltText('A boot'))
    expect(screen.queryByAltText('A boot')).toBeNull()
    expect(broken.container.querySelector('svg.lucide-image')).not.toBeNull()
  })

  it('fetches nothing for a private picture the server cannot resize and is too big to send whole', async () => {
    const fetching = vi.fn(async () => new Response(new Blob(['gif'])))
    vi.stubGlobal('fetch', fetching)
    const { container } = render(<AssetImage asset={{ ...boot, id: 'hidden-reel', visibility: 'PRIVATE', mimeType: 'image/gif', sizeBytes: 40 << 20, url: '', thumbnailUrl: '' }} />)
    await new Promise((r) => setTimeout(r, 20))
    expect(fetching).not.toHaveBeenCalled()
    expect(screen.queryByAltText('A boot')).toBeNull()
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('asks for a private picture only once its tile comes near the screen', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:kept')
    const fetching = vi.fn(async () => new Response(new Blob(['jpg'])))
    vi.stubGlobal('fetch', fetching)
    let report: (near: boolean) => void = () => {}
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(seen: IntersectionObserverCallback) {
          report = (near) => seen([{ isIntersecting: near } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
        }
        observe() {}
        disconnect() {}
      },
    )
    render(<AssetImage asset={{ ...boot, id: 'far-down', visibility: 'PRIVATE', url: '', thumbnailUrl: '' }} />)
    await new Promise((r) => setTimeout(r, 20))
    expect(fetching).not.toHaveBeenCalled()

    act(() => report(true))
    expect(((await screen.findByAltText('A boot')) as HTMLImageElement).src).toBe('blob:kept')
    expect(fetching.mock.calls.map((c) => (c as unknown[])[0])).toEqual([`${ADMIN_MEDIA_FILE_ENDPOINT}/far-down?w=640`])
  })
})
