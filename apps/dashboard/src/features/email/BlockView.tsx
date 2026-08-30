import { Fragment, type CSSProperties, type ReactNode } from 'react'
import { EyeOffIcon } from 'lucide-react'
import {
  SAMPLE_LINES,
  SAMPLE_SHIPTO,
  interpolateSample,
  isParameterized,
  type BlockComponent,
  type BlockStyle,
  type EmailBlock,
  type EmailStyles,
} from './blocks'

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
        <h2 style={{ margin: '0 0 8px', fontSize: s.fontSize || 20, textAlign: align(s), color: s.color || theme.textColor || '#222' }}>
          {interpolateSample(block.text ?? '') || 'Heading'}
        </h2>
      )
    case 'text':
      return (
        <p style={{ margin: '0 0 12px', lineHeight: 1.5, textAlign: align(s), color: s.color || theme.textColor || '#333', fontSize: s.fontSize || undefined }}>
          {multiline(interpolateSample(block.content ?? '') || 'Text block')}
        </p>
      )
    case 'button':
      return (
        <div style={{ textAlign: align(s), margin: '16px 0' }}>
          <span style={{ display: 'inline-block', background: s.buttonColor || theme.linkColor || '#222', color: s.buttonTextColor || '#fff', padding: '10px 18px', borderRadius: s.radius ?? 4, fontSize: 14 }}>
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
    case 'products': {
      const p = block.products
      const cols = Math.max(1, p?.columns || 3)
      const count =
        p?.mode === 'collection' ? Math.max(1, Math.min(cols, p?.limit || cols)) : Math.max(1, (p?.skus?.length || cols))
      const label =
        p?.mode === 'collection'
          ? `Collection · up to ${p?.limit ?? 0} products`
          : `${p?.skus?.length ?? 0} product${(p?.skus?.length ?? 0) === 1 ? '' : 's'} by SKU`
      const cards = Math.min(count, cols)
      return (
        <div style={{ margin: '12px 0' }}>
          <div style={{ marginBottom: 6, fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: '#0d7d5b' }}>
            Products - {label}
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: `repeat(${cols}, 1fr)`, gap: 8 }}>
            {Array.from({ length: cards }).map((_, i) => (
              <div key={i} style={{ border: '1px dashed #b6dccb', background: '#f2fbf7', borderRadius: 4, padding: 8, textAlign: 'center' }}>
                <div style={{ height: 56, background: '#e4f4ec', borderRadius: 3, marginBottom: 6 }} />
                <div style={{ fontSize: 11, color: '#4b8a72' }}>Product</div>
              </div>
            ))}
          </div>
          <div style={{ marginTop: 4, fontSize: 11, color: '#999' }}>Resolves to live products on send.</div>
        </div>
      )
    }
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
    case 'divider':
      return <hr style={{ border: 'none', borderTop: `1px solid ${s.color || '#e5e5e5'}`, margin: '16px 0' }} />
    case 'spacer':
      return <div style={{ height: block.size || 16 }} />
    case 'order_items':
      return (
        <table cellPadding={6} style={{ borderCollapse: 'collapse', width: '100%', fontSize: 14 }}>
          <thead>
            <tr style={{ textAlign: 'left', borderBottom: '1px solid #ddd' }}>
              <th>Item</th>
              <th>Qty</th>
              <th style={{ textAlign: 'right' }}>Total</th>
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
              ['Subtotal', interpolateSample('{{order.subtotal}}')],
              ['Discount', `-${interpolateSample('{{order.discount}}')}`],
              ['Shipping', interpolateSample('{{order.shipping}}')],
              ['Tax', interpolateSample('{{order.tax}}')],
            ].map(([label, val]) => (
              <tr key={label}>
                <td>{label}</td>
                <td style={{ textAlign: 'right' }}>{val}</td>
              </tr>
            ))}
            <tr>
              <td>
                <strong>Total</strong>
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
          <strong>Shipping to</strong>
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
          Carrier: {interpolateSample('{{tracking.carrier}}')} &middot; Tracking:{' '}
          <strong>{interpolateSample('{{tracking.number}}')}</strong>
        </p>
      )
    case 'refund_amount':
      return (
        <p style={{ fontSize: 14 }}>
          Refund issued: <strong>{interpolateSample('{{refund.amount}}')}</strong>
        </p>
      )
    default:
      return null
  }
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
}: {
  block: EmailBlock
  theme: EmailStyles
  components?: Record<string, BlockComponent>
  renderColumn?: (colIndex: number) => ReactNode
}) {
  return (
    <ConditionalWrap visibleIf={block.visibleIf}>
      <BlockContent block={block} theme={theme} components={components} renderColumn={renderColumn} />
    </ConditionalWrap>
  )
}

function BlockContent({
  block,
  theme,
  components,
  renderColumn,
}: {
  block: EmailBlock
  theme: EmailStyles
  components?: Record<string, BlockComponent>
  renderColumn?: (colIndex: number) => ReactNode
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

  return <>{box(block.style, <Leaf block={block} theme={theme} />)}</>
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
