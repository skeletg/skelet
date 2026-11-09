import { describe, expect, it } from 'vitest'
import { emailFacts, formatBytes, previewDocument } from './previewDoc'

// What these pin: links open in a new tab, dark mode inverts all but the pictures, and the checks catch dead links and
// images without alt text
describe('the preview document', () => {
  it('adds the preview head into the email, dark only when asked', () => {
    const html = '<!DOCTYPE html><html><head><style>x{}</style></head><body>Hi</body></html>'
    const light = previewDocument(html)
    expect(light).toContain('<head><base target="_blank">')
    expect(light).not.toContain('invert')
    const dark = previewDocument(html, { dark: true })
    expect(dark).toContain('html{filter:invert(1) hue-rotate(180deg)')
    expect(dark).toContain('img,picture,video')
    expect(previewDocument('<p>Bare</p>')).toMatch(/^<!DOCTYPE html><html><head><base target="_blank">.*<body><p>Bare<\/p><\/body>/)
  })

  it('lists every link and flags the ones that go nowhere', () => {
    const facts = emailFacts(
      '<a href="https://shop.test/sale">Sale</a><a href="#">Top</a><a href="javascript:x()">Bad</a><a href="">  </a>' +
        '<a href="mailto:hi@shop.test"><img src="x.png" alt="Write us"></a><img src="y.png">',
    )
    expect(facts.links.map((l) => [l.text, l.ok])).toEqual([
      ['Sale', true],
      ['Top', false],
      ['Bad', false],
      ['(no text)', false],
      ['(image)', true],
    ])
    expect(facts.images).toBe(2)
    expect(facts.imagesWithoutAlt).toBe(1)
  })

  it('counts bytes, not characters', () => {
    expect(emailFacts('é').bytes).toBe(2)
    expect(formatBytes(900)).toBe('900 B')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(150 * 1024)).toBe('150 KB')
  })
})
