import { describe, expect, it } from 'vitest'
import type { CmsBlock } from './page-blocks'
import { firstBlockImage, runSeoLint, truncate, type LintInput, type LintResult } from './seo-analysis'

// The panel runs these against the editor's UNSAVED state, so what matters is that a check reads
// the same thing an author is looking at, and that the body-image scan agrees with the backend's
// (graph/front/cms_helpers.go) about which image a page falls back to

const block = (type: string, props: Record<string, unknown> = {}, blocks?: CmsBlock[]): CmsBlock => ({
  id: type + Math.random().toString(36).slice(2, 6),
  type,
  props,
  ...(blocks ? { blocks } : {}),
})

const asset = (url: string) => ({ type: 'asset', assetId: 'a1', url })

const input = (over: Partial<LintInput> = {}): LintInput => ({
  title: '',
  metaTitle: '',
  metaDescription: '',
  slug: '',
  seoNoindex: false,
  blocks: [],
  ...over,
})

const find = (out: LintResult[], id: string) => out.find((r) => r.id === id)
const level = (out: LintResult[], id: string) => find(out, id)?.level

describe('truncate', () => {
  it('leaves anything short enough alone, trimmed', () => {
    expect(truncate('  hello  ', 20)).toBe('hello')
  })

  it('cuts to the limit including the ellipsis', () => {
    const out = truncate('abcdefghij', 5)
    expect(out).toBe('abcd…')
    expect(out).toHaveLength(5)
  })

  it('does not leave a space stranded before the ellipsis', () => {
    // the cut lands on the space, and 'ab …' would read as a typo
    expect(truncate('ab cdefgh', 4)).toBe('ab…')
  })
})

describe('the title check', () => {
  it('fails when there is nothing to show at all', () => {
    expect(level(runSeoLint(input()), 'title')).toBe('fail')
  })

  it('falls back to the page title when no meta title is set', () => {
    const out = runSeoLint(input({ title: 'A'.repeat(40) }))
    expect(level(out, 'title')).toBe('pass')
  })

  // the meta title is what search results show, so it outranks the page title
  it('prefers the meta title over the page title', () => {
    const out = runSeoLint(input({ title: 'A'.repeat(40), metaTitle: 'short' }))
    expect(level(out, 'title')).toBe('warn')
    expect(find(out, 'title')?.detail).toContain('5 chars')
  })

  it('warns either side of the 30-60 window and passes on the boundaries', () => {
    expect(level(runSeoLint(input({ metaTitle: 'A'.repeat(29) })), 'title')).toBe('warn')
    expect(level(runSeoLint(input({ metaTitle: 'A'.repeat(30) })), 'title')).toBe('pass')
    expect(level(runSeoLint(input({ metaTitle: 'A'.repeat(60) })), 'title')).toBe('pass')
    expect(level(runSeoLint(input({ metaTitle: 'A'.repeat(61) })), 'title')).toBe('warn')
  })

  it('counts a whitespace-only title as none', () => {
    expect(level(runSeoLint(input({ metaTitle: '   ' })), 'title')).toBe('fail')
  })
})

describe('the description check', () => {
  it('fails when empty and warns either side of 120-160', () => {
    expect(level(runSeoLint(input()), 'desc')).toBe('fail')
    expect(level(runSeoLint(input({ metaDescription: 'A'.repeat(119) })), 'desc')).toBe('warn')
    expect(level(runSeoLint(input({ metaDescription: 'A'.repeat(120) })), 'desc')).toBe('pass')
    expect(level(runSeoLint(input({ metaDescription: 'A'.repeat(160) })), 'desc')).toBe('pass')
    expect(level(runSeoLint(input({ metaDescription: 'A'.repeat(161) })), 'desc')).toBe('warn')
  })
})

describe('the rest of the checklist', () => {
  it('only mentions noindex when the page is actually excluded', () => {
    expect(find(runSeoLint(input()), 'noindex')).toBeUndefined()
    expect(level(runSeoLint(input({ seoNoindex: true })), 'noindex')).toBe('warn')
  })

  it('says nothing about heroes until there is more than one', () => {
    expect(find(runSeoLint(input({ blocks: [block('hero')] })), 'hero')).toBeUndefined()
    const out = runSeoLint(input({ blocks: [block('hero'), block('hero')] }))
    expect(level(out, 'hero')).toBe('warn')
    expect(find(out, 'hero')?.detail).toContain('2 hero')
  })

  it('counts heroes nested inside other blocks', () => {
    const out = runSeoLint(input({ blocks: [block('hero'), block('section', {}, [block('hero')])] }))
    expect(level(out, 'hero')).toBe('warn')
  })

  it('warns only on a slug past 60 chars', () => {
    expect(find(runSeoLint(input({ slug: 'a'.repeat(60) })), 'slug')).toBeUndefined()
    expect(level(runSeoLint(input({ slug: 'a'.repeat(61) })), 'slug')).toBe('warn')
  })
})

describe('the alt-text check', () => {
  it('passes a page with no images at all', () => {
    expect(level(runSeoLint(input({ blocks: [block('text')] })), 'alt')).toBe('pass')
  })

  it('flags an image whose alt field is blank', () => {
    const out = runSeoLint(input({ blocks: [block('image', { imageUrl: '/a.png', alt: '  ' })] }))
    expect(level(out, 'alt')).toBe('warn')
    expect(find(out, 'alt')?.detail).toContain('1 image')
  })

  it('passes once the alt is filled in', () => {
    expect(level(runSeoLint(input({ blocks: [block('image', { imageUrl: '/a.png', alt: 'A cat' })] })), 'alt')).toBe('pass')
  })

  // a decorative hero background has no alt field, so the editor has nothing to fix
  it('ignores an image in a scope with no alt field to set', () => {
    expect(level(runSeoLint(input({ blocks: [block('hero', { backgroundImage: '/bg.png' })] })), 'alt')).toBe('pass')
  })

  it('looks inside repeater items too', () => {
    const gallery = block('gallery', { items: [{ src: '/1.png', alt: '' }, { src: '/2.png', alt: 'ok' }] })
    expect(find(runSeoLint(input({ blocks: [gallery] })), 'alt')?.detail).toContain('1 image')
  })

  it('counts an asset-ref image the same as a plain url', () => {
    const out = runSeoLint(input({ blocks: [block('image', { imageUrl: asset('/a.png'), alt: '' })] }))
    expect(level(out, 'alt')).toBe('warn')
  })
})

describe('firstBlockImage', () => {
  it('finds nothing on a page with no images', () => {
    expect(firstBlockImage([block('text')])).toBe('')
  })

  it('takes the first block that has one, in document order', () => {
    expect(firstBlockImage([block('text'), block('image', { imageUrl: '/a.png' }), block('image', { imageUrl: '/b.png' })])).toBe('/a.png')
  })

  // the key order is the fallback priority the backend uses, so both agree on the og:image
  it('prefers imageUrl over the later keys on one block', () => {
    expect(firstBlockImage([block('x', { src: '/src.png', imageUrl: '/first.png' })])).toBe('/first.png')
  })

  it('unwraps an asset ref', () => {
    expect(firstBlockImage([block('image', { imageUrl: asset('/asset.png') })])).toBe('/asset.png')
  })

  it('descends into child blocks', () => {
    expect(firstBlockImage([block('section', {}, [block('image', { src: '/deep.png' })])])).toBe('/deep.png')
  })

  it('looks inside repeater item arrays', () => {
    expect(firstBlockImage([block('gallery', { items: [{ src: '/item.png' }] })])).toBe('/item.png')
  })

  it('ignores a blank or whitespace-only url', () => {
    expect(firstBlockImage([block('image', { imageUrl: '   ' }), block('image', { src: '/real.png' })])).toBe('/real.png')
  })

  // a page that somehow nests forever must not take the editor down with it
  it('gives up past the depth ceiling instead of recursing forever', () => {
    let deep = block('image', { src: '/bottom.png' })
    for (let i = 0; i < 12; i++) deep = block('section', {}, [deep])
    expect(firstBlockImage([deep])).toBe('')
  })
})
