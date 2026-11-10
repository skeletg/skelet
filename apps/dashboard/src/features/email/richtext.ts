// Rebuilds markup from the rich text subset only - the server cuts every send back to the same one (emailtemplate richtext.go)

const TAGS: Record<string, string> = {
  P: 'p',
  BR: 'br',
  STRONG: 'strong',
  B: 'strong',
  EM: 'em',
  I: 'em',
  U: 'u',
  S: 's',
  STRIKE: 's',
  DEL: 's',
  UL: 'ul',
  OL: 'ol',
  LI: 'li',
  A: 'a',
}
const INLINE_FORMATS = new Set(['strong', 'em', 'u', 's'])
const DROPPED = new Set(['SCRIPT', 'STYLE', 'TEMPLATE', 'NOSCRIPT', 'IFRAME', 'OBJECT', 'EMBED', 'SVG', 'MATH', 'HEAD', 'TITLE', 'TEXTAREA', 'SELECT'])
const MERGE_TAG = /\{\{\s*([\w.]+)\s*\}\}/g

function safeHref(href: string): boolean {
  const h = href.trim()
  return /^(https?:|mailto:)/i.test(h) || /^\{\{\s*[\w.]+\s*\}\}/.test(h)
}

// The editor marks underline and strikethrough as a span's inline style
function styleFormats(el: Element): string[] {
  const style = (el.getAttribute('style') ?? '').toLowerCase()
  const out: string[] = []
  if (/font-weight:\s*(bold|[6-9]00)/.test(style)) out.push('strong')
  if (/font-style:\s*italic/.test(style)) out.push('em')
  if (/text-decoration[^;]*underline/.test(style)) out.push('u')
  if (/text-decoration[^;]*line-through/.test(style)) out.push('s')
  return out
}

function copyInto(from: Node, to: Element, doc: Document, open: Set<string>) {
  for (const node of Array.from(from.childNodes)) {
    if (node.nodeType === Node.TEXT_NODE) {
      to.appendChild(doc.createTextNode(node.textContent ?? ''))
      continue
    }
    if (node.nodeType !== Node.ELEMENT_NODE) continue
    const el = node as Element
    if (DROPPED.has(el.tagName.toUpperCase())) continue
    const tag = TAGS[el.tagName]
    if (!tag) {
      // Unknown wrappers (span, div, font...) go, their text stays - keeping any formatting their style asked for
      let target = to
      const added: string[] = []
      for (const f of styleFormats(el)) {
        if (open.has(f)) continue
        const wrap = doc.createElement(f)
        target.appendChild(wrap)
        target = wrap
        added.push(f)
      }
      added.forEach((f) => open.add(f))
      copyInto(el, target, doc, open)
      added.forEach((f) => open.delete(f))
      continue
    }
    if (INLINE_FORMATS.has(tag) && open.has(tag)) {
      copyInto(el, to, doc, open)
      continue
    }
    if (tag === 'a' && !safeHref(el.getAttribute('href') ?? '')) {
      copyInto(el, to, doc, open)
      continue
    }
    const copy = doc.createElement(tag)
    if (tag === 'a') copy.setAttribute('href', (el.getAttribute('href') ?? '').trim())
    if (INLINE_FORMATS.has(tag)) open.add(tag)
    copyInto(el, copy, doc, open)
    if (INLINE_FORMATS.has(tag)) open.delete(tag)
    to.appendChild(copy)
  }
}

function parse(html: string): HTMLElement {
  return new DOMParser().parseFromString(`<!doctype html><body>${html}</body>`, 'text/html').body
}

export function normalizeRichHtml(html: string): string {
  const doc = document.implementation.createHTMLDocument('')
  const root = doc.createElement('div')
  copyInto(parse(html), root, doc, new Set())
  // A trailing empty paragraph is the editor's caret line, not content
  let last = root.lastElementChild
  while (last && last.tagName === 'P' && (last.textContent ?? '').trim() === '' && !last.querySelector('a')) {
    last.remove()
    last = root.lastElementChild
  }
  return root.innerHTML
}

function escapeText(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function plainToRich(text: string): string {
  const paragraphs = text.replace(/\r\n/g, '\n').split(/\n{2,}/).filter((p) => p.trim() !== '')
  if (paragraphs.length === 0) return ''
  return paragraphs.map((p) => `<p>${escapeText(p).replace(/\n/g, '<br>')}</p>`).join('')
}

export function richToPlain(html: string): string {
  const body = parse(normalizeRichHtml(html))
  body.querySelectorAll('br').forEach((br) => br.replaceWith('\n'))
  body.querySelectorAll('li').forEach((li) => li.prepend('- '))
  return Array.from(body.children)
    .map((el) => (el.tagName === 'UL' || el.tagName === 'OL' ? Array.from(el.children).map((li) => li.textContent).join('\n') : el.textContent))
    .join('\n\n')
    .trim()
}

// Built from DOM nodes out of the subset, so it is safe to set as HTML
export function richToDisplayHtml(html: string, sample: (tag: string) => string): string {
  const doc = document.implementation.createHTMLDocument('')
  const root = doc.createElement('div')
  root.innerHTML = normalizeRichHtml(html)
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const texts: Text[] = []
  for (let n = walker.nextNode(); n; n = walker.nextNode()) texts.push(n as Text)
  for (const t of texts) {
    const value = t.textContent ?? ''
    if (!MERGE_TAG.test(value)) continue
    MERGE_TAG.lastIndex = 0
    const frag = doc.createDocumentFragment()
    let at = 0
    for (const m of value.matchAll(MERGE_TAG)) {
      frag.append(value.slice(at, m.index))
      const chip = doc.createElement('span')
      chip.setAttribute('data-merge-tag', m[1])
      chip.className = 'rounded bg-info-soft px-1 text-info'
      chip.textContent = sample(m[1])
      frag.append(chip)
      at = (m.index ?? 0) + m[0].length
    }
    frag.append(value.slice(at))
    t.replaceWith(frag)
  }
  for (const a of Array.from(root.querySelectorAll('a'))) a.setAttribute('style', 'color:var(--email-link, inherit);text-decoration:underline')
  return root.innerHTML
}
