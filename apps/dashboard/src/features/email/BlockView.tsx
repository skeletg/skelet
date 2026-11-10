import { Fragment, type CSSProperties, type ReactNode } from 'react'
import { EyeOffIcon } from 'lucide-react'
import {
  blockWord,
  buttonStyle,
  headingStyle,
  mergeTagLabel,
  RICH_TEXT_CLASSES,
  textBlockStyle,
  SAMPLE_LINES,
  SAMPLE_VARS,
  SAMPLE_SHIPTO,
  interpolateSample,
  isParameterized,
  type BlockComponent,
  type BlockStyle,
  type EmailBlock,
  type EmailStyles,
} from './blocks'
import { productImage, useBlockProducts } from './blockProducts'
import { richToDisplayHtml } from './richtext'

function sampleOf(tag: string): string {
  return SAMPLE_VARS[tag] || mergeTagLabel(tag)
}

// Client-side approximation of the Go renderer (internal/emailtemplate) so the canvas can show blocks as they'll roughly look - with sample
// data + merge tags resolved - Not byte-identical to the sent email (the server preview is authoritative); close enough to design against

function align(s?: BlockStyle): CSSProperties['textAlign'] {
  return s?.align ?? 'left'
}

// socialName maps a platform key to a display label (mirrors server socialLabel)
const SOCIAL_NAMES: Record<string, string> = {
  facebook: 'Facebook',
  instagram: 'Instagram',
  x: 'X',
  twitter: 'X',
  youtube: 'YouTube',
  linkedin: 'LinkedIn',
  tiktok: 'TikTok',
  pinterest: 'Pinterest',
}
function socialName(p: string): string {
  return SOCIAL_NAMES[p] ?? p
}

function multiline(text: string): ReactNode {
  const parts = text.split('\n')
  return parts.map((line, i) => (
    <Fragment key={i}>
      {i > 0 && <br />}
      {line}
    </Fragment>
  ))
}

// box applies block-level padding + background (mirrors Go boxHTML), and flags
// blocks hidden on mobile with a small badge so the operator sees the rule
function box(style: BlockStyle | undefined, children: ReactNode): ReactNode {
  if (!style) return children
  const css: CSSProperties = {}
  if (style.paddingY || style.paddingX) css.padding = `${style.paddingY ?? 0}px ${style.paddingX ?? 0}px`
  if (style.background) css.background = style.background
  const wrapped = Object.keys(css).length === 0 ? children : <div style={css}>{children}</div>
  if (!style.hideMobile) return wrapped
  return (
    <div style={{ position: 'relative' }}>
      <div
        title="Hidden on mobile viewports"
        style={{ position: 'absolute', top: -8, left: 6, zIndex: 1, display: 'inline-flex', alignItems: 'center', gap: 3, background: '#e0e7ff', color: '#4338ca', border: '1px solid #c7d2fe', borderRadius: 999, padding: '0 6px', fontSize: 9, fontWeight: 600 }}
      >
        <EyeOffIcon size={9} /> mobile
      </div>
      {wrapped}
    </div>
  )
}

function Leaf({ block, theme }: { block: EmailBlock; theme: EmailStyles }) {
  const s = block.style ?? {}
  switch (block.type) {
    case 'heading':
      return (
        <h2 style={headingStyle(block, theme)}>
          {interpolateSample(block.text ?? '') || 'Heading'}
        </h2>
      )
    case 'text':
      if (block.format === 'rich') {
        return (
          <div
            className={RICH_TEXT_CLASSES}
            style={{ ...textBlockStyle(block, theme), margin: '0 0 12px', ['--email-link' as string]: theme.linkColor || undefined }}
            // built node by node from the allowed subset (richtext.ts), never the stored string itself
            dangerouslySetInnerHTML={{ __html: richToDisplayHtml(block.content ?? '', sampleOf) || '<p>Text block</p>' }}
          />
        )
      }
      return (
        <p style={{ margin: '0 0 12px', lineHeight: 1.5, textAlign: align(s), color: s.color || theme.textColor || '#333', fontSize: s.fontSize || undefined }}>
          {multiline(interpolateSample(block.content ?? '') || 'Text block')}
        </p>
      )
    case 'button':
      return (
        <div style={{ textAlign: align(s), margin: '16px 0' }}>
          <span style={buttonStyle(block, theme)}>
            {interpolateSample(block.label ?? '') || 'Button'}
          </span>
        </div>
      )
    case 'image': {
      const src = interpolateSample(block.url ?? '').trim()
      if (!src) {
        return (
          <div style={{ textAlign: align(s), margin: '12px 0' }}>
            <div style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: s.width ? `${s.width}%` : 160, height: 80, background: '#f0f0f0', color: '#999', fontSize: 12, border: '1px dashed #ccc' }}>
              Image
            </div>
          </div>
        )
      }
      return (
        <div style={{ textAlign: align(s), margin: '12px 0' }}>
          <img src={src} alt={interpolateSample(block.alt ?? '')} style={{ width: s.width ? `${s.width}%` : undefined, maxWidth: '100%', height: 'auto' }} />
        </div>
      )
    }
    case 'products':
      return <ProductsLeaf block={block} />
    case 'social': {
      const links = block.social ?? []
      return (
        <div style={{ textAlign: align(s), margin: '12px 0' }}>
          {links.length === 0 ? (
            <span style={{ fontSize: 12, color: '#999' }}>No social links yet</span>
          ) : (
            links.map((l, i) => (
              <span
                key={i}
                style={{ display: 'inline-block', margin: '0 6px', padding: '4px 10px', borderRadius: 999, background: '#eef1f4', color: '#4b5563', fontSize: 12, fontWeight: 600, textTransform: 'capitalize' }}
              >
                {socialName(l.platform)}
              </span>
            ))
          )}
        </div>
      )
    }
    case 'video': {
      const poster = interpolateSample(block.poster ?? '').trim()
      return (
        <div style={{ textAlign: align(s), margin: '12px 0', position: 'relative' }}>
          <div style={{ position: 'relative', display: 'inline-block', maxWidth: '100%' }}>
            {poster ? (
              <img src={poster} alt={interpolateSample(block.alt ?? '')} style={{ maxWidth: '100%', height: 'auto', borderRadius: 8, display: 'block' }} />
            ) : (
              <div style={{ display: 'grid', placeItems: 'center', width: 240, height: 135, background: '#e5e7eb', color: '#9ca3af', fontSize: 12, borderRadius: 8 }}>
                Video thumbnail
              </div>
            )}
            <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', fontSize: 40, color: '#fff', textShadow: '0 1px 4px rgba(0,0,0,.6)' }}>▶</div>
          </div>
        </div>
      )
    }
    case 'countdown': {
      const dl = interpolateSample(block.deadline ?? '').trim()
      return (
        <div style={{ textAlign: align(s), margin: '16px 0', fontWeight: 700, color: s.color || theme.textColor || '#222' }}>
          {interpolateSample(block.label ?? '')}{' '}
          <span style={{ fontSize: s.fontSize || 22 }}>{dl || <span style={{ color: '#999', fontWeight: 400, fontSize: 13 }}>(set a deadline)</span>}</span>
        </div>
      )
    }
    case 'html':
      return (
        <div style={{ margin: '8px 0', border: '1px dashed #cbd5e1', background: '#f8fafc', borderRadius: 4, padding: 10 }}>
          <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: '#64748b', marginBottom: 4 }}>Raw HTML</div>
          <pre style={{ margin: 0, fontSize: 11, color: '#475569', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 120, overflow: 'hidden' }}>
            {(block.content ?? '').slice(0, 400) || '<!-- empty -->'}
          </pre>
          <div style={{ marginTop: 4, fontSize: 11, color: '#94a3b8' }}>Sanitized + rendered on send.</div>
        </div>
      )
    case 'coupon':
      return (
        <div style={{ margin: '16px 0', padding: 20, border: `2px dashed ${s.buttonColor || theme.linkColor || '#222'}`, borderRadius: 8, textAlign: s.align ?? 'center', color: s.color || theme.textColor || '#222' }}>
          {block.text ? <div style={{ fontSize: 16, fontWeight: 700, margin: '0 0 10px' }}>{interpolateSample(block.text)}</div> : null}
          <div style={{ display: 'inline-block', fontFamily: "'Courier New', Courier, monospace", fontSize: 24, fontWeight: 700, letterSpacing: 3, padding: '8px 16px', background: '#f4f4f5', borderRadius: 4 }}>
            {interpolateSample(block.code ?? '') || 'CODE'}
          </div>
          {block.content ? <div style={{ fontSize: 13, color: '#666', marginTop: 10 }}>{interpolateSample(block.content)}</div> : null}
          {block.label && block.url ? (
            <div style={{ marginTop: 14 }}>
              <span style={buttonStyle(block, theme)}>{interpolateSample(block.label)}</span>
            </div>
          ) : null}
        </div>
      )
    case 'image_text': {
      const src = interpolateSample(block.image ?? '').trim()
      const picture = (
        <div style={{ flex: '0 0 45%' }}>
          {src ? (
            <img src={src} alt={interpolateSample(block.alt ?? '')} style={{ display: 'block', width: '100%', height: 'auto', borderRadius: 4 }} />
          ) : (
            <div style={{ aspectRatio: '4 / 3', display: 'grid', placeItems: 'center', background: '#f0f0f0', color: '#999', fontSize: 12, border: '1px dashed #ccc', borderRadius: 4 }}>
              Image
            </div>
          )}
        </div>
      )
      const words = (
        <div style={{ flex: 1, minWidth: 0, textAlign: align(s), color: s.color || theme.textColor || '#333' }}>
          {block.text ? <h3 style={{ margin: '0 0 8px', fontSize: 18, fontWeight: 700 }}>{interpolateSample(block.text)}</h3> : null}
          {block.content ? <p style={{ margin: '0 0 12px', lineHeight: 1.5 }}>{multiline(interpolateSample(block.content))}</p> : null}
          {block.label && block.url ? <span style={buttonStyle(block, theme)}>{interpolateSample(block.label)}</span> : null}
        </div>
      )
      return (
        <div className="email-stack" style={{ display: 'flex', gap: 16, alignItems: 'center', margin: '12px 0' }}>
          {block.side === 'right' ? (
            <>
              {words}
              {picture}
            </>
          ) : (
            <>
              {picture}
              {words}
            </>
          )}
        </div>
      )
    }
    case 'menu': {
      const links = (block.links ?? []).filter((l) => l.label.trim())
      return (
        <div style={{ margin: '12px 0', textAlign: s.align ?? 'center' }}>
          {links.length === 0 ? (
            <span style={{ fontSize: 12, color: '#999' }}>No links yet</span>
          ) : (
            links.map((l, i) => (
              <Fragment key={i}>
                {i > 0 ? <span style={{ color: '#ccc', padding: '0 10px' }}>|</span> : null}
                <span style={{ color: s.color || theme.linkColor || '#222', fontWeight: 600, fontSize: 14 }}>{interpolateSample(l.label)}</span>
              </Fragment>
            ))
          )}
        </div>
      )
    }
    case 'store_footer':
      return (
        <div style={{ margin: '24px 0 0', paddingTop: 16, borderTop: '1px solid #eee', textAlign: s.align ?? 'center', fontSize: 12, lineHeight: 1.6, color: s.color || '#888' }}>
          {block.content ? multiline(interpolateSample(block.content)) : null}
          <div style={{ marginTop: 8 }}>
            <span style={{ textDecoration: 'underline' }}>{block.label?.trim() || 'Unsubscribe'}</span>
          </div>
        </div>
      )
    case 'divider':
      return <hr style={{ border: 'none', borderTop: `1px solid ${s.color || '#e5e5e5'}`, margin: '16px 0' }} />
    case 'spacer':
      return <div style={{ height: block.size || 16 }} />
    case 'order_items':
      return (
        <table cellPadding={6} style={{ borderCollapse: 'collapse', width: '100%', fontSize: 14 }}>
          <thead>
            <tr style={{ textAlign: 'left', borderBottom: '1px solid #ddd' }}>
              <th>{blockWord(block, 'item')}</th>
              <th>{blockWord(block, 'qty')}</th>
              <th style={{ textAlign: 'right' }}>{blockWord(block, 'total')}</th>
            </tr>
          </thead>
          <tbody>
            {SAMPLE_LINES.map((l) => (
              <tr key={l.sku} style={{ borderBottom: '1px solid #f0f0f0' }}>
                <td>
                  {l.name}
                  <br />
                  <span style={{ color: '#888', fontSize: 12 }}>{l.sku}</span>
                </td>
                <td>{l.qty}</td>
                <td style={{ textAlign: 'right' }}>{l.total}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )
    case 'order_totals':
      return (
        <table style={{ marginTop: 12, width: '100%', fontSize: 14 }}>
          <tbody>
            {[
              [blockWord(block, 'subtotal'), interpolateSample('{{order.subtotal}}')],
              [blockWord(block, 'discount'), `-${interpolateSample('{{order.discount}}')}`],
              [blockWord(block, 'shipping'), interpolateSample('{{order.shipping}}')],
              [blockWord(block, 'tax'), interpolateSample('{{order.tax}}')],
            ].map(([label, val]) => (
              <tr key={label}>
                <td>{label}</td>
                <td style={{ textAlign: 'right' }}>{val}</td>
              </tr>
            ))}
            <tr>
              <td>
                <strong>{blockWord(block, 'total')}</strong>
              </td>
              <td style={{ textAlign: 'right' }}>
                <strong>{interpolateSample('{{order.total}}')}</strong>
              </td>
            </tr>
          </tbody>
        </table>
      )
    case 'shipping_address':
      return (
        <p style={{ marginTop: 12, fontSize: 14 }}>
          <strong>{blockWord(block, 'shippingTo')}</strong>
          <br />
          {SAMPLE_SHIPTO.map((line) => (
            <Fragment key={line}>
              {line}
              <br />
            </Fragment>
          ))}
        </p>
      )
    case 'tracking':
      return (
        <p style={{ fontSize: 14 }}>
          {blockWord(block, 'carrier')}: {interpolateSample('{{tracking.carrier}}')} &middot; {blockWord(block, 'tracking')}:{' '}
          <strong>{interpolateSample('{{tracking.number}}')}</strong>
        </p>
      )
    case 'refund_amount':
      return (
        <p style={{ fontSize: 14 }}>
          {blockWord(block, 'refund')}: <strong>{interpolateSample('{{refund.amount}}')}</strong>
        </p>
      )
    default:
      return null
  }
}

function ProductsLeaf({ block }: { block: EmailBlock }) {
  const p = block.products
  const byCollection = p?.mode === 'collection'
  const skus = byCollection ? [] : (p?.skus ?? [])
  const { bySku } = useBlockProducts(skus)
  const cols = Math.max(1, p?.columns || 3)
  const count = byCollection ? Math.max(1, Math.min(cols, p?.limit || cols)) : Math.max(1, skus.length || cols)
  const label = byCollection ? `Collection · up to ${p?.limit ?? 0} products` : `${Math.min(skus.length, p?.limit && p.limit > 0 ? p.limit : 3)} product${skus.length === 1 ? '' : 's'}`
  // A send shows up to the limit - the canvas shows the same cards
  const limit = p?.limit && p.limit > 0 ? p.limit : 3
  const cards = byCollection || skus.length === 0 ? Array.from({ length: Math.min(count, cols) }, () => undefined) : skus.slice(0, limit).map((sku) => bySku.get(sku))
  return (
    <div style={{ margin: '12px 0' }}>
      <div style={{ marginBottom: 6, fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: '#0d7d5b' }}>
        Products - {label}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: `repeat(${Math.min(cols, 4)}, minmax(0, 1fr))`, gap: 8 }}>
        {cards.map((product, i) => {
          const image = productImage(product)
          return (
            <div key={i} style={{ border: '1px solid #e5e7eb', background: '#fff', borderRadius: 4, padding: 8, textAlign: 'center' }}>
              {image ? (
                <img src={image} alt="" style={{ display: 'block', width: '100%', aspectRatio: '1', objectFit: 'cover', borderRadius: 3, marginBottom: 6 }} />
              ) : (
                <div style={{ aspectRatio: '1', background: '#f2f4f5', borderRadius: 3, marginBottom: 6 }} />
              )}
              <div style={{ fontSize: 12, color: '#333', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{product?.name ?? 'Product'}</div>
            </div>
          )
        })}
      </div>
      <div style={{ marginTop: 4, fontSize: 11, color: '#999' }}>Prices and stock come in live on send.</div>
    </div>
  )
}

/**
 * BlockView renders a single block (leaf or row) for the WYSIWYG canvas - Row
 * blocks render their columns side by side; an optional `renderColumn` slot
 * lets the canvas inject editable drop zones (Phase 3) instead of the static
 * read-only column rendering used for refs/preview
 */
export function BlockView({
  block,
  theme,
  components,
  renderColumn,
  editor,
}: {
  block: EmailBlock
  theme: EmailStyles
  components?: Record<string, BlockComponent>
  renderColumn?: (colIndex: number) => ReactNode
  editor?: ReactNode
}) {
  return (
    <ConditionalWrap visibleIf={block.visibleIf}>
      <BlockContent block={block} theme={theme} components={components} renderColumn={renderColumn} editor={editor} />
    </ConditionalWrap>
  )
}

function BlockContent({
  block,
  theme,
  components,
  renderColumn,
  editor,
}: {
  block: EmailBlock
  theme: EmailStyles
  components?: Record<string, BlockComponent>
  renderColumn?: (colIndex: number) => ReactNode
  editor?: ReactNode
}) {
  if (block.type === 'ref') {
    const comp = block.slug ? components?.[block.slug] : undefined
    if (!comp) {
      return (
        <div style={{ padding: 12, border: '1px dashed #c0b3e0', background: '#f7f4fc', color: '#6b4ea8', fontSize: 13, borderRadius: 4 }}>
          Reusable block: {block.slug || '(none selected)'}
        </div>
      )
    }
    // Parameterized / HTML-body blocks render per-instance on send - show a labelled card with the instance's field
    // values - Plain snippets render their composed body inline
    if (isParameterized(comp)) {
      return <ComponentCard comp={comp} fields={block.fields} />
    }
    return (
      <>
        {comp.blocks.map((b, i) => (
          <BlockView key={i} block={b} theme={theme} components={components} />
        ))}
      </>
    )
  }

  if (block.type === 'row') {
    const cols = block.columns ?? []
    return (
      <div style={{ display: 'flex', gap: 12, margin: '8px 0' }}>
        {cols.map((c, i) => {
          const w = c.width && c.width > 0 ? c.width : 1
          return (
            <div key={i} style={{ flex: `${w} ${w} 0`, minWidth: 0 }}>
              {renderColumn
                ? renderColumn(i)
                : (c.blocks ?? []).map((cb, bi) => (
                    <BlockView key={bi} block={cb} theme={theme} components={components} />
                  ))}
            </div>
          )
        })}
      </div>
    )
  }

  // A non-built-in type names a component slug - render it as a card
  const comp = components?.[block.type]
  if (comp) {
    return <ComponentCard comp={comp} fields={block.fields} />
  }

  return <>{box(block.style, editor ?? <Leaf block={block} theme={theme} />)}</>
}

// Human-readable summary of a visibleIf rule for the canvas badge
const OP_LABELS: Record<string, string> = { exists: 'exists', eq: '=', ne: '≠', contains: 'contains' }
function conditionSummary(v: NonNullable<EmailBlock['visibleIf']>): string {
  const field = v.field || '(field)'
  const op = OP_LABELS[v.op] ?? v.op
  const base = v.op === 'exists' ? `${field} ${op}` : `${field} ${op} ${v.value ?? ''}`.trim()
  return v.negate ? `NOT (${base})` : base
}

// ConditionalWrap decorates a block that has a visibility condition with a
// subtle badge so the operator sees, on the canvas, that it's gated at send
function ConditionalWrap({ visibleIf, children }: { visibleIf?: EmailBlock['visibleIf']; children: ReactNode }) {
  if (!visibleIf) return <>{children}</>
  return (
    <div style={{ position: 'relative', border: '1px dashed #d9b310', borderRadius: 4, padding: '2px' }}>
      <div
        title={`Only shown if: ${conditionSummary(visibleIf)}`}
        style={{ display: 'inline-flex', alignItems: 'center', gap: 4, position: 'absolute', top: -9, right: 6, background: '#fef3c7', color: '#92600a', border: '1px solid #f0d074', borderRadius: 999, padding: '1px 7px', fontSize: 10, fontWeight: 600, zIndex: 1 }}
      >
        <EyeOffIcon size={10} />
        Conditional
      </div>
      {children}
    </div>
  )
}

// ComponentCard previews a parameterized / HTML-body block on the canvas: its name + the instance's filled field values
// - The authoritative render (the composed/HTML body) happens server-side
function ComponentCard({ comp, fields }: { comp: BlockComponent; fields?: Record<string, string> }) {
  const filled = comp.fields
    .map((f) => ({ label: f.label || f.key, val: interpolateSample(fields?.[f.key] ?? '') }))
    .filter((x) => x.val.trim() !== '')
  return (
    <div style={{ padding: 12, border: '1px dashed #cdb4f6', background: '#faf7ff', borderRadius: 4, margin: '8px 0' }}>
      <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: '#7c4dcc' }}>
        {comp.icon ? `${comp.icon} ` : ''}
        {comp.name}
      </div>
      {filled.length > 0 ? (
        <div style={{ marginTop: 6, fontSize: 13, color: '#444' }}>
          {filled.map((x) => (
            <div key={x.label}>
              <span style={{ color: '#888' }}>{x.label}: </span>
              {x.val}
            </div>
          ))}
        </div>
      ) : (
        <div style={{ marginTop: 6, fontSize: 12, color: '#999' }}>Reusable block - rendered on send.</div>
      )}
    </div>
  )
}
