// Gmail cuts a message off past this many bytes of HTML, behind a "View entire message" link
export const GMAIL_CLIP_BYTES = 102 * 1024

export const SIZE_WARN_SHARE = 0.75

export function sizeLevel(bytes: number): 'ok' | 'warn' | 'over' {
  if (bytes > GMAIL_CLIP_BYTES) return 'over'
  return bytes > GMAIL_CLIP_BYTES * SIZE_WARN_SHARE ? 'warn' : 'ok'
}

export function previewDocument(html: string, opts: { dark?: boolean } = {}): string {
  const head = [
    // A document is never shorter than its frame unless the root sizes to its content - this is what gets measured
    '<base target="_blank"><style>html{height:auto!important}body{min-height:0!important}</style>',
    opts.dark
      ? '<style>html{filter:invert(1) hue-rotate(180deg);background:#fff}img,picture,video,[style*="background-image"]{filter:invert(1) hue-rotate(180deg)}</style>'
      : '',
  ].join('')
  if (/<head[^>]*>/i.test(html)) return html.replace(/<head[^>]*>/i, (m) => m + head)
  return `<!DOCTYPE html><html><head>${head}</head><body>${html}</body></html>`
}

export type PreviewLink = { text: string; href: string; ok: boolean }

export type EmailFacts = {
  bytes: number
  links: PreviewLink[]
  images: number
  imagesWithoutAlt: number
}

function linkOk(href: string): boolean {
  return /^(https?:\/\/[^\s/]+|mailto:\S+@\S+|tel:\S+)/i.test(href.trim())
}

export function emailFacts(html: string): EmailFacts {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const links = Array.from(doc.querySelectorAll('a')).map((a) => {
    const href = a.getAttribute('href') ?? ''
    const text = (a.textContent ?? '').replace(/\s+/g, ' ').trim() || (a.querySelector('img') ? '(image)' : '(no text)')
    return { text, href, ok: linkOk(href) }
  })
  const imgs = Array.from(doc.querySelectorAll('img'))
  return {
    bytes: new TextEncoder().encode(html).length,
    links,
    images: imgs.length,
    imagesWithoutAlt: imgs.filter((i) => !(i.getAttribute('alt') ?? '').trim()).length,
  }
}

export function formatBytes(n: number): string {
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`
}
