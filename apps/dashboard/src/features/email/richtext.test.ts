import { describe, expect, it } from 'vitest'
import { normalizeRichHtml, plainToRich, richToDisplayHtml, richToPlain } from './richtext'

// What these pin: only the subset survives, plain text never turns into markup, and merge tags show their samples
describe('rich text', () => {
  it('keeps the subset and drops everything else', () => {
    const editorExport =
      '<p dir="ltr" class="x"><b><strong class="b" style="white-space: pre-wrap;">Bold</strong></b>' +
      '<span style="text-decoration: underline;">under</span><i><em>it</em></i></p>' +
      '<p onclick="x()">Go <a href="javascript:alert(1)">bad</a> <a href="{{storefront.url}}/sale" target="_blank">sale</a></p>' +
      '<script>alert(1)</script><ul><li><span>one</span></li></ul><p><br></p>'
    expect(normalizeRichHtml(editorExport)).toBe(
      '<p><strong>Bold</strong><u>under</u><em>it</em></p><p>Go bad <a href="{{storefront.url}}/sale">sale</a></p><ul><li>one</li></ul>',
    )
  })

  it('turns plain text into paragraphs without reading it as markup', () => {
    expect(plainToRich('Hi <b>there</b>\nline two\n\nNew paragraph')).toBe('<p>Hi &lt;b&gt;there&lt;/b&gt;<br>line two</p><p>New paragraph</p>')
    expect(plainToRich('  \n\n ')).toBe('')
  })

  it('reads back as plain text', () => {
    expect(richToPlain('<p>Hi<br>there</p><ul><li>a</li><li>b</li></ul>')).toBe('Hi\nthere\n\n- a\n- b')
  })

  it('shows merge tags as their sample values', () => {
    const out = richToDisplayHtml('<p>Hi {{customer.firstName}} <img src=x onerror=alert(1)></p>', (tag) => (tag === 'customer.firstName' ? 'Alex' : tag))
    expect(out).toContain('<span data-merge-tag="customer.firstName"')
    expect(out).toContain('>Alex</span>')
    expect(out).not.toContain('onerror')
    expect(out).not.toContain('<img')
  })
})
