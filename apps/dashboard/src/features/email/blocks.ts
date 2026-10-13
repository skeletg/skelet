// Typed-block model shared by the email-template editor and the reusable block library - Mirrors the server-side block
// registry in internal/emailtemplate/blocks.go - keep the type list + fields in sync

export type BlockStyle = {
  align?: 'left' | 'center' | 'right'
  color?: string
  fontSize?: number
  paddingY?: number
  paddingX?: number
  background?: string
  buttonColor?: string
  buttonTextColor?: string
  radius?: number
  width?: number
  /** Hidden on narrow viewports via a media query - mirrors server-side BlockStyle.hideMobile */
  hideMobile?: boolean
}

/** One entry of a `social` block - Mirrors server-side emailtemplate.SocialLink */
export type SocialLink = { platform: string; url: string }

/** Social platforms the `social` block knows how to label */
export const SOCIAL_PLATFORMS = [
  'facebook',
  'instagram',
  'x',
  'youtube',
  'linkedin',
  'tiktok',
  'pinterest',
] as const

export type Column = {
  width?: number
  blocks?: EmailBlock[]
}

export type EmailBlock = {
  type: string
  text?: string
  content?: string
  label?: string
  url?: string
  alt?: string
  size?: number
  slug?: string
  style?: BlockStyle
  columns?: Column[]
  /** Custom block-type instances keep field values here, keyed by the type's field `key` */
  fields?: Record<string, string>
  /** Renders only when the merge-tag `field` satisfies `op` (exists/eq/ne/contains) against `value`, with `negate` inverting the whole test */
  visibleIf?: { field: string; op: string; value?: string; negate?: boolean }
  /** Product-block config - resolves to live products at send time, canvas shows a placeholder */
  products?: { mode: string; skus?: string[]; collectionId?: string; limit?: number; columns?: number }
  /** Social links (type === 'social') - Mirrors server-side Block.social */
  social?: SocialLink[]
  /** Thumbnail image URL for a `video` block, whose url is the video link */
  poster?: string
  /** Target time text for a `countdown` block, where label is the prefix */
  deadline?: string
}

export type EmailStyles = {
  background?: string
  contentWidth?: number
  fontFamily?: string
  linkColor?: string
  textColor?: string
}

export type BlockFieldKind = 'text' | 'textarea' | 'url' | 'number' | 'ref' | 'image'

export type BlockField = {
  key: keyof EmailBlock
  label: string
  kind: BlockFieldKind
  /** Field accepts {{merge.tags}} from the palette */
  supportsTags?: boolean
  placeholder?: string
}

export type BlockDescriptor = {
  type: string
  label: string
  hint: string
  fields: BlockField[]
  /** Group the block falls under in the "Add block" menu */
  group: 'content' | 'data' | 'layout' | 'reusable'
}

// The catalog. `data` blocks render only when the email's context supplies
// their data (e.g. order_items in an order email); they take no config
export const BLOCK_TYPES: BlockDescriptor[] = [
  {
    type: 'heading',
    label: 'Heading',
    hint: 'A bold section heading.',
    group: 'content',
    fields: [{ key: 'text', label: 'Heading text', kind: 'text', supportsTags: true }],
  },
  {
    type: 'text',
    label: 'Text',
    hint: 'A paragraph of body copy.',
    group: 'content',
    fields: [{ key: 'content', label: 'Text', kind: 'textarea', supportsTags: true }],
  },
  {
    type: 'button',
    label: 'Button',
    hint: 'A call-to-action link styled as a button.',
    group: 'content',
    fields: [
      { key: 'label', label: 'Button label', kind: 'text', supportsTags: true },
      { key: 'url', label: 'Link URL', kind: 'url', supportsTags: true, placeholder: 'https://…' },
    ],
  },
  {
    type: 'image',
    label: 'Image',
    hint: 'An inline image referenced by URL.',
    group: 'content',
    fields: [
      { key: 'url', label: 'Image', kind: 'image', supportsTags: true, placeholder: 'https://… or {{store.logoUrl}}' },
      { key: 'alt', label: 'Alt text', kind: 'text', supportsTags: true },
    ],
  },
  {
    type: 'products',
    label: 'Products',
    hint: 'A grid of products (by SKU or collection). Resolves to live products on send.',
    group: 'content',
    fields: [],
  },
  {
    type: 'social',
    label: 'Social links',
    hint: 'A row of icon links to your social profiles.',
    group: 'content',
    fields: [],
  },
  {
    type: 'video',
    label: 'Video',
    hint: 'A thumbnail that links out to a video (email can’t embed video).',
    group: 'content',
    fields: [
      { key: 'url', label: 'Video URL', kind: 'url', supportsTags: true, placeholder: 'https://youtu.be/…' },
      { key: 'poster', label: 'Thumbnail image', kind: 'image', supportsTags: true, placeholder: 'https://…' },
      { key: 'alt', label: 'Alt text', kind: 'text', supportsTags: true },
    ],
  },
  {
    type: 'countdown',
    label: 'Countdown',
    hint: 'A deadline banner (e.g. “Sale ends July 5”). Static, not animated.',
    group: 'content',
    fields: [
      { key: 'label', label: 'Prefix', kind: 'text', supportsTags: true, placeholder: 'Offer ends' },
      { key: 'deadline', label: 'Deadline text', kind: 'text', supportsTags: true, placeholder: 'July 5, 2026' },
    ],
  },
  {
    type: 'html',
    label: 'Raw HTML',
    hint: 'Custom HTML (sanitized on send - no scripts). For advanced layouts.',
    group: 'content',
    fields: [{ key: 'content', label: 'HTML', kind: 'textarea', supportsTags: true, placeholder: '<div>…</div>' }],
  },
  { type: 'divider', label: 'Divider', hint: 'A horizontal rule.', group: 'content', fields: [] },
  {
    type: 'spacer',
    label: 'Spacer',
    hint: 'Vertical empty space.',
    group: 'content',
    fields: [{ key: 'size', label: 'Height (px)', kind: 'number', placeholder: '16' }],
  },
  {
    type: 'order_items',
    label: 'Order items',
    hint: 'The order line-item table. Renders only in order emails.',
    group: 'data',
    fields: [],
  },
  {
    type: 'order_totals',
    label: 'Order totals',
    hint: 'Subtotal / shipping / tax / total. Order emails only.',
    group: 'data',
    fields: [],
  },
  {
    type: 'shipping_address',
    label: 'Shipping address',
    hint: 'The order shipping address. Order emails only.',
    group: 'data',
    fields: [],
  },
  {
    type: 'tracking',
    label: 'Tracking',
    hint: 'Carrier + tracking number. Shipment emails only.',
    group: 'data',
    fields: [],
  },
  {
    type: 'refund_amount',
    label: 'Refund amount',
    hint: 'The refunded total. Refund emails only.',
    group: 'data',
    fields: [],
  },
  {
    type: 'row',
    label: 'Columns',
    hint: 'A multi-column row. Drop blocks into each column.',
    group: 'layout',
    fields: [],
  },
  {
    type: 'ref',
    label: 'Reusable block',
    hint: 'Embeds a block from the reusable library.',
    group: 'reusable',
    fields: [{ key: 'slug', label: 'Library block', kind: 'ref' }],
  },
]

// --- Reusable blocks: the unified component ------------------------------ Mirrors emailtemplate.Snippet - optionally parameterized, with
// either a composed body or an admin-authored HTML one (sanitized server-side), embedded in emails as a `ref`

export type CustomFieldKind =
  | 'text'
  | 'textarea'
  | 'url'
  | 'color'
  | 'number'
  | 'date'
  | 'image'
  | 'select'
  | 'boolean'

export type CustomFieldDef = {
  key: string
  label: string
  kind: CustomFieldKind
  required?: boolean
  default?: string
  /** For kind === 'select': the allowed values */
  options?: string[]
}

export type BlockComponent = {
  slug: string
  name: string
  icon?: string
  fields: CustomFieldDef[]
  blocks: EmailBlock[]
  htmlTemplate?: string
  /** Custom CSS for the HTML body, injected as a sanitized <style> at render */
  css?: string
  /** Render the HTML body through Liquid (conditionals / loops / filters) */
  scripted?: boolean
}

/** Configured per-use or rendered from an HTML body, vs a plain snippet embedded verbatim */
export function isParameterized(c: BlockComponent): boolean {
  return c.fields.length > 0 || !!c.htmlTemplate?.trim()
}

/** Builds a quick lookup of components by slug */
export function componentMap(list: BlockComponent[]): Record<string, BlockComponent> {
  return Object.fromEntries(list.map((c) => [c.slug, c]))
}

/** newBlockRef builds a ref to a component, seeding each field with its default */
export function newBlockRef(c: BlockComponent): EmailBlock {
  if (c.fields.length === 0) return { type: 'ref', slug: c.slug }
  const fields: Record<string, string> = {}
  for (const f of c.fields) fields[f.key] = f.default ?? ''
  return { type: 'ref', slug: c.slug, fields }
}

const BY_TYPE: Record<string, BlockDescriptor> = Object.fromEntries(
  BLOCK_TYPES.map((b) => [b.type, b]),
)

export function blockDescriptor(type: string): BlockDescriptor | undefined {
  return BY_TYPE[type]
}

export function blockLabel(type: string): string {
  return BY_TYPE[type]?.label ?? type
}

// newBlock builds a block of the given type with sensible defaults
export function newBlock(type: string): EmailBlock {
  switch (type) {
    case 'heading':
      return { type, text: 'Heading' }
    case 'text':
      return { type, content: 'Your message here.' }
    case 'button':
      return { type, label: 'View order', url: '{{storefront.url}}' }
    case 'image':
      return { type, url: '{{store.logoUrl}}', alt: '{{store.name}}' }
    case 'spacer':
      return { type, size: 16 }
    case 'ref':
      return { type, slug: '' }
    case 'row':
      return { type, columns: [{ width: 1, blocks: [] }, { width: 1, blocks: [] }] }
    case 'products':
      return { type, products: { mode: 'skus', skus: [], limit: 3, columns: 3 } }
    case 'social':
      return { type, social: [{ platform: 'facebook', url: '' }, { platform: 'instagram', url: '' }] }
    case 'video':
      return { type, url: '', poster: '', alt: '' }
    case 'countdown':
      return { type, label: 'Offer ends', deadline: '' }
    case 'html':
      return { type, content: '<!-- your HTML -->' }
    default:
      return { type }
  }
}

// SAMPLE_VARS mirror the server's preview sample data (internal/emailtemplate
// admin.go sampleFor) so the canvas shows realistic interpolated copy
export const SAMPLE_VARS: Record<string, string> = {
  'store.name': 'Your Store',
  'store.logoUrl': '',
  'customer.firstName': 'Alex',
  'storefront.url': 'https://example.com',
  'order.number': 'ORD-1001',
  'order.subtotal': '65.00 USD',
  'order.discount': '5.00 USD',
  'order.shipping': '7.00 USD',
  'order.tax': '5.20 USD',
  'order.total': '72.20 USD',
  'tracking.carrier': 'UPS',
  'tracking.number': '1Z999AA10123456784',
  'refund.amount': '72.20 USD',
  'verify.link': 'https://example.com/verify',
  'reset.link': 'https://example.com/reset',
  'reset.expiresIn': '30 minutes',
  'confirm.link': 'https://example.com/subscribe/confirm/sample',
  'confirm.expiresIn': '7 days',
  'signin.link': 'https://example.com/account/login',
  'forgot.link': 'https://example.com/account/forgot',
}

export const SAMPLE_LINES = [
  { name: 'Sample Tee', sku: 'TEE-1', qty: 2, total: '50.00 USD' },
  { name: 'Sample Mug', sku: 'MUG-1', qty: 1, total: '15.00 USD' },
]
export const SAMPLE_SHIPTO = ['Alex Doe', '123 Main St', 'Austin TX 78701', 'US']

// interpolateSample replaces {{token}} with its sample value (unknown → '')
// Mirrors the Go interpolate lexer closely enough for the canvas
export function interpolateSample(s: string, vars: Record<string, string> = SAMPLE_VARS): string {
  return s.replace(/\{\{\s*([\w.]+)\s*\}\}/g, (_, tag: string) => vars[tag] ?? '')
}

// escapeHtml mirrors Go's html.EscapeString - used when substituting untrusted
// values into a custom type's admin-authored HTML template
export function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&#34;')
    .replace(/'/g, '&#39;')
}

// Substitutes {{token}} with the HTML-escaped value, leaving the template's own HTML intact - a client-side approximation of the server's
// version for the type designer's live preview, where the authoritative render and template sanitizing stay server-side
export function interpolateEscaped(tmpl: string, vars: Record<string, string>): string {
  return tmpl.replace(/\{\{\s*([\w.]+)\s*\}\}/g, (_, tag: string) => escapeHtml(vars[tag] ?? ''))
}

// shortPreview renders a one-line summary of a block for the collapsed list row
export function shortPreview(b: EmailBlock): string {
  switch (b.type) {
    case 'heading':
      return b.text ?? ''
    case 'text':
      return (b.content ?? '').replace(/\s+/g, ' ').slice(0, 80)
    case 'button':
      return `${b.label ?? ''} → ${b.url ?? ''}`
    case 'image':
      return b.url ?? ''
    case 'spacer':
      return `${b.size ?? 0}px`
    case 'ref':
      return b.slug ? `@${b.slug}` : '(no block selected)'
    case 'row':
      return `${b.columns?.length ?? 0} columns`
    case 'products': {
      const p = b.products
      if (!p) return 'Products'
      return p.mode === 'collection'
        ? `Collection (up to ${p.limit ?? 0})`
        : `${p.skus?.length ?? 0} SKUs`
    }
    case 'social':
      return `${b.social?.length ?? 0} link${(b.social?.length ?? 0) === 1 ? '' : 's'}`
    case 'video':
      return b.url ?? ''
    case 'countdown':
      return `${b.label ?? ''} ${b.deadline ?? ''}`.trim()
    case 'html':
      return (b.content ?? '').replace(/\s+/g, ' ').slice(0, 80)
    default:
      return ''
  }
}
