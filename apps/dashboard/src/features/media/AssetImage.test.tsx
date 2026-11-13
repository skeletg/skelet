import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AssetImage } from './AssetImage'

const boot = { url: 'https://cdn/boot.jpg', thumbnailUrl: 'https://cdn/derived/w640/boot.jpg', altText: 'A boot' }

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
})
