// CMS component-based page builder - the typed block model + the editor's
// palette/inspector descriptors - This is the dashboard third of a contract
// shared THREE ways and MUST stay in lockstep:
//
// SYNC SENTINEL:
//   - backend validator      → apps/backend/internal/blockkit (Block / Document /
//                              BuiltinTypes - the authoritative type list)
//   - storefront renderer     → apps/channels/vetrina-demo/app/components/cms (parseDocument
//                              + the CmsBlocks registry in Section.tsx)
//   - dashboard builder        → THIS FILE (CMS_BLOCK_TYPES) + PageBlockCanvas
//   - enforced by             → bin/contract-check (all three lists, every CI run)
//
// A page body that parses to a `{ version, blocks }` Document is a block-built
// page; anything else (Lexical or plain rich text) is left untouched by the
// builder - Mirrors the email-template block model (features/email/blocks.ts)
// but FLAT - an ordered list of blocks, no nested containers or columns

import {
  HeadingIcon,
  MegaphoneIcon,
  CodeIcon,
  FormInputIcon,
  PackageIcon,
  LayersIcon,
  FolderTreeIcon,
  NewspaperIcon,
  SquareStackIcon,
  Columns3Icon,
  TypeIcon,
  ListCollapseIcon,
  FoldVerticalIcon,
  VideoIcon,
  ImagesIcon,
  MinusIcon,
  PuzzleIcon,
  QuoteIcon,
  BarChart3Icon,
  InfoIcon,
  MousePointerClickIcon,
  GalleryThumbnailsIcon,
  LayoutGridIcon,
  GalleryHorizontalIcon,
  MilestoneIcon,
  LayoutDashboardIcon,
  TruckIcon,
  type LucideIcon,
} from 'lucide-react'
import { randomId } from '@/lib/random-id'

// One node in a page layout. `props` are type-specific (rendered per type;
// stored/transported as-is). `blocks` are children for container types - kept
// optional so a nested tree round-trips without loss, but unused in v1 (flat)
// Mirrors blockkit.Block and the storefront Block type exactly
export type CmsBlock = {
  id: string
  type: string
  props: Record<string, unknown>
  blocks?: CmsBlock[]
}

// The shape persisted in cms_page_content.body for a block-built page - Mirrors
// blockkit.Document and the storefront Document type
export type CmsDocument = {
  version: number
  blocks: CmsBlock[]
}

export const CMS_DOCUMENT_VERSION = 1

// A field's editing widget. 'image' is a URL input with a thumbnail preview;
// 'collection'/'category' are entity pickers populated from the catalog
export type CmsFieldKind =
  | 'text'
  | 'textarea'
  | 'url'
  | 'link' // internal-reference picker OR custom URL (see LinkField)
  | 'number'
  | 'select'
  | 'image'
  | 'collection'
  | 'category'
  | 'variant' // visual-look picker - options come from BLOCK_VARIANTS[blockType]
  | 'richtext' // Lexical rich-text editor (value stored as Lexical JSON)
  | 'repeater' // array of items, each editing the field's itemFields
  | 'snippet' // picker of reusable-section slugs (for the `ref` block)
  | 'form' // picker of custom web forms (for the `form` block)

// CmsLinkRef is the stored value of a 'link' field when it points at an internal entity (vs a plain string = custom
// URL) - The backend resolves (entityType, entityId) to the live, channel-aware URL at render; `label` is editor-only
export type CmsLinkRef = {
  type: 'reference'
  entityType: string
  entityId: string
  label?: string
}

export function isLinkRef(v: unknown): v is CmsLinkRef {
  return (
    typeof v === 'object' && v !== null &&
    (v as { type?: unknown }).type === 'reference' &&
    typeof (v as { entityId?: unknown }).entityId === 'string'
  )
}

export type CmsBlockField = {
  /** Key under the block's `props` bag */
  key: string
  label: string
  kind: CmsFieldKind
  required?: boolean
  /** Allowed values for kind === 'select' */
  options?: Array<{ value: string; label: string }>
  placeholder?: string
  /** Help text shown under the input */
  description?: string
  /** Only show this field when a sibling prop equals a value (conditional UI) */
  showIf?: { key: string; equals: string }
  /** For kind === 'variant': which BLOCK_REGISTRIES option-set to choose from */
  registry?: string
  /** For kind === 'variant': render a searchable dropdown (good for long,
   *  extensible lists) instead of the inline option cards */
  searchable?: boolean
  /** For kind === 'repeater': the sub-fields edited per array item */
  itemFields?: CmsBlockField[]
  /** Default item-count hint for an empty repeater (cosmetic) */
  addLabel?: string
}

export type CmsBlockDescriptor = {
  type: string
  label: string
  hint: string
  icon: LucideIcon
  fields: CmsBlockField[]
  /** Container blocks hold child blocks (edited via a nested canvas) instead of
   *  (or alongside) leaf fields */
  container?: boolean
}

// The v1 catalog - Every `type` here is a built-in in blockkit.BuiltinTypes; the storefront renders each from its props - Dynamic grids
// (product/collection/ category/blog) take a `source`/`limit` descriptor in props and the storefront fetches their data
export const CMS_BLOCK_TYPES: CmsBlockDescriptor[] = [
  {
    type: 'hero',
    label: 'Hero',
    hint: 'A headline banner with an optional image and call-to-action.',
    icon: HeadingIcon,
    fields: [
      { key: 'eyebrow', label: 'Eyebrow', kind: 'text', placeholder: 'e.g. New season', description: 'Small pill above the heading (with a status dot).' },
      { key: 'heading', label: 'Heading', kind: 'text', required: true },
      { key: 'subheading', label: 'Subheading', kind: 'textarea' },
      { key: 'imageUrl', label: 'Background image', kind: 'image', placeholder: 'https://…', description: 'Blurred behind the heading.' },
      { key: 'ctaLabel', label: 'Primary button label', kind: 'text' },
      { key: 'ctaHref', label: 'Primary button link', kind: 'link', description: 'Pick a page/product/category (link tracks it through renames) or enter a custom URL.' },
      { key: 'cta2Label', label: 'Secondary button label', kind: 'text' },
      { key: 'cta2Href', label: 'Secondary button link', kind: 'link' },
      {
        key: 'showSearch',
        label: 'Search bar',
        kind: 'select',
        options: [
          { value: 'none', label: 'Hidden' },
          { value: 'show', label: 'Catalog search + popular categories' },
        ],
        description: 'Renders the storefront search pill inside the hero.',
      },
      { key: 'goalEvent', label: 'A/B goal event', kind: 'text', description: 'When the page is A/B tested, a CTA click fires this event - match it to the experiment’s goal.' },
      {
        key: 'align',
        label: 'Alignment',
        kind: 'select',
        options: [
          { value: 'left', label: 'Left' },
          { value: 'center', label: 'Center' },
        ],
      },
    ],
  },
  {
    type: 'cta_banner',
    label: 'CTA banner',
    hint: 'A compact call-to-action strip.',
    icon: MegaphoneIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text', required: true },
      { key: 'text', label: 'Text', kind: 'textarea' },
      { key: 'ctaLabel', label: 'Button label', kind: 'text' },
      { key: 'ctaHref', label: 'Button link', kind: 'link' },
      { key: 'goalEvent', label: 'A/B goal event', kind: 'text', description: 'When the page is A/B tested, a click fires this event - match it to the experiment’s goal.' },
    ],
  },
  {
    type: 'html',
    label: 'Custom HTML',
    hint: 'A raw HTML block (sanitized server-side on render).',
    icon: CodeIcon,
    fields: [{ key: 'html', label: 'HTML', kind: 'textarea', placeholder: '<section>…</section>' }],
  },
  {
    type: 'form',
    label: 'Form',
    hint: 'Embed a custom form. Submissions land under CMS → Forms.',
    icon: FormInputIcon,
    fields: [{ key: 'formId', label: 'Form', kind: 'form', description: 'Pick a form to embed. Manage forms under CMS → Forms.' }],
  },
  {
    type: 'product_grid',
    label: 'Product grid',
    hint: 'A grid of products - newest, or from a collection/category.',
    icon: PackageIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'variant', label: 'Layout', kind: 'variant', registry: 'layout', description: 'How the products are arranged.' },
      { key: 'card', label: 'Card style', kind: 'variant', registry: 'product_card', searchable: true, description: 'How each product is presented.' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true, description: 'The section frame, background & heading.' },
      {
        key: 'source',
        label: 'Source',
        kind: 'select',
        required: true,
        options: [
          { value: 'newest', label: 'Newest products' },
          { value: 'collection', label: 'From a collection' },
          { value: 'category', label: 'From a category' },
        ],
      },
      { key: 'collectionId', label: 'Collection', kind: 'collection', showIf: { key: 'source', equals: 'collection' } },
      { key: 'categoryId', label: 'Category', kind: 'category', showIf: { key: 'source', equals: 'category' } },
      { key: 'limit', label: 'How many', kind: 'number', placeholder: '8', description: 'Max products to show.' },
      {
        key: 'onlySale',
        label: 'Products',
        kind: 'select',
        options: [
          { value: 'all', label: 'All products' },
          { value: 'sale', label: 'On sale only' },
        ],
        description: 'On sale = compare-at price beats the sell price.',
      },
    ],
  },
  {
    type: 'collection_grid',
    label: 'Collection grid',
    hint: 'A grid of storefront collections.',
    icon: LayersIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'variant', label: 'Layout', kind: 'variant', registry: 'layout', description: 'How the collections are arranged.' },
      { key: 'card', label: 'Card style', kind: 'variant', registry: 'collection_card', searchable: true, description: 'How each collection is presented.' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true, description: 'The section frame, background & heading.' },
      { key: 'limit', label: 'Limit', kind: 'number', placeholder: '6' },
    ],
  },
  {
    type: 'category_grid',
    label: 'Category grid',
    hint: 'A grid of catalog categories.',
    icon: FolderTreeIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'variant', label: 'Layout', kind: 'variant', registry: 'layout', description: 'How the categories are arranged.' },
      { key: 'card', label: 'Card style', kind: 'variant', registry: 'category_card', searchable: true, description: 'How each category is presented.' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true, description: 'The section frame, background & heading.' },
      { key: 'limit', label: 'Limit', kind: 'number', placeholder: '6' },
    ],
  },
  {
    type: 'blog_grid',
    label: 'Blog grid',
    hint: 'A grid of recent blog posts.',
    icon: NewspaperIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'limit', label: 'Limit', kind: 'number', placeholder: '6' },
    ],
  },
  {
    type: 'value_strip',
    label: 'Value strip',
    hint: 'A slim band of store promises - shipping, returns, support.',
    icon: TruckIcon,
    fields: [
      {
        key: 'items', label: 'Promises', kind: 'repeater', addLabel: 'Add promise',
        description: 'Leave empty for the storefront defaults.',
        itemFields: [
          {
            key: 'icon', label: 'Icon', kind: 'select',
            options: [
              { value: 'shipping', label: 'Shipping' },
              { value: 'returns', label: 'Returns' },
              { value: 'secure', label: 'Secure checkout' },
              { value: 'support', label: 'Support' },
            ],
          },
          { key: 'title', label: 'Title', kind: 'text', required: true },
          { key: 'text', label: 'Subtext', kind: 'text' },
        ],
      },
    ],
  },
  {
    type: 'promo_mosaic',
    label: 'Promo mosaic',
    hint: 'Auto-built merchandising bento: deals splash, category, new arrivals and a collection.',
    icon: LayoutDashboardIcon,
    fields: [],
  },
  // ---- layout containers (hold child blocks) ----
  {
    type: 'section',
    label: 'Section',
    hint: 'A full-width band wrapping other blocks, with a background & spacing.',
    icon: SquareStackIcon,
    container: true,
    fields: [
      {
        key: 'background', label: 'Background', kind: 'select',
        options: [
          { value: 'none', label: 'None' },
          { value: 'muted', label: 'Muted (shaded band)' },
          { value: 'foreground', label: 'Inverted (dark)' },
        ],
      },
      {
        key: 'width', label: 'Width', kind: 'select',
        options: [
          { value: 'normal', label: 'Normal' },
          { value: 'wide', label: 'Wide' },
          { value: 'full', label: 'Full bleed' },
        ],
      },
      {
        key: 'padding', label: 'Vertical padding', kind: 'select',
        options: [
          { value: 'normal', label: 'Normal' },
          { value: 'large', label: 'Large' },
          { value: 'none', label: 'None' },
        ],
      },
    ],
  },
  {
    type: 'columns',
    label: 'Columns',
    hint: 'Lays child blocks into a responsive multi-column grid.',
    icon: Columns3Icon,
    container: true,
    fields: [
      {
        key: 'count', label: 'Columns', kind: 'select',
        options: [
          { value: '2', label: '2 columns' },
          { value: '3', label: '3 columns' },
          { value: '4', label: '4 columns' },
        ],
      },
    ],
  },
  // ---- content blocks ----
  {
    type: 'rich_text',
    label: 'Rich text',
    hint: 'Formatted prose - headings, lists, links, images.',
    icon: TypeIcon,
    fields: [{ key: 'content', label: 'Content', kind: 'richtext' }],
  },
  {
    type: 'accordion',
    label: 'Accordion / FAQ',
    hint: 'A list of expandable question/answer items.',
    icon: ListCollapseIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      {
        key: 'items', label: 'Items', kind: 'repeater', addLabel: 'Add item',
        itemFields: [
          { key: 'heading', label: 'Question / title', kind: 'text', required: true },
          { key: 'body', label: 'Answer / body', kind: 'textarea' },
        ],
      },
    ],
  },
  {
    type: 'tabs',
    label: 'Tabs',
    hint: 'Switchable labeled panels.',
    icon: FoldVerticalIcon,
    fields: [
      {
        key: 'items', label: 'Tabs', kind: 'repeater', addLabel: 'Add tab',
        itemFields: [
          { key: 'label', label: 'Tab label', kind: 'text', required: true },
          { key: 'body', label: 'Panel content', kind: 'textarea' },
        ],
      },
    ],
  },
  {
    type: 'video',
    label: 'Video',
    hint: 'An embedded YouTube/Vimeo or direct video file.',
    icon: VideoIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'url', label: 'Video URL', kind: 'url', required: true, placeholder: 'https://youtube.com/watch?v=… or .mp4' },
    ],
  },
  {
    type: 'gallery',
    label: 'Image gallery',
    hint: 'A responsive grid of images.',
    icon: ImagesIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      {
        key: 'columns', label: 'Columns', kind: 'select',
        options: [
          { value: '2', label: '2' },
          { value: '3', label: '3' },
          { value: '4', label: '4' },
        ],
      },
      {
        key: 'items', label: 'Images', kind: 'repeater', addLabel: 'Add image',
        itemFields: [
          { key: 'url', label: 'Image', kind: 'image', required: true },
          { key: 'alt', label: 'Alt text', kind: 'text' },
          // plain URL (not a 'link' ref): repeater-item props aren't reached by
          // the server-side link resolver, which only scans top-level block props
          { key: 'href', label: 'Links to (URL)', kind: 'url' },
        ],
      },
    ],
  },
  {
    type: 'ref',
    label: 'Reusable section',
    hint: 'Embed a saved Section - edit it once, updates everywhere.',
    icon: PuzzleIcon,
    fields: [
      { key: 'slug', label: 'Section', kind: 'snippet', required: true, description: 'Which reusable section to embed.' },
    ],
  },
  {
    type: 'divider',
    label: 'Divider / spacer',
    hint: 'A horizontal rule or blank vertical space.',
    icon: MinusIcon,
    fields: [
      {
        key: 'style', label: 'Style', kind: 'select',
        options: [
          { value: 'line', label: 'Line' },
          { value: 'blank', label: 'Blank space' },
        ],
      },
      {
        key: 'size', label: 'Size', kind: 'select',
        options: [
          { value: 'sm', label: 'Small' },
          { value: 'md', label: 'Medium' },
          { value: 'lg', label: 'Large' },
        ],
      },
    ],
  },
  // ---------- second content/marketing pack (static) ----------
  {
    type: 'quote',
    label: 'Quote / testimonial',
    hint: 'A pull-quote or customer testimonial with optional author + avatar.',
    icon: QuoteIcon,
    fields: [
      { key: 'quote', label: 'Quote', kind: 'textarea', required: true },
      { key: 'author', label: 'Author', kind: 'text' },
      { key: 'role', label: 'Role / company', kind: 'text' },
      { key: 'avatar', label: 'Avatar', kind: 'image' },
      { key: 'variant', label: 'Style', kind: 'variant', registry: 'quote_style' },
    ],
  },
  {
    type: 'stats',
    label: 'Stats / metrics',
    hint: 'A row of big-number metrics.',
    icon: BarChart3Icon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'cols', label: 'Columns', kind: 'variant', registry: 'cols' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true },
      {
        key: 'items', label: 'Stats', kind: 'repeater', addLabel: 'Add stat',
        itemFields: [
          { key: 'value', label: 'Value', kind: 'text', required: true },
          { key: 'label', label: 'Label', kind: 'text' },
          { key: 'description', label: 'Description', kind: 'text' },
        ],
      },
    ],
  },
  {
    type: 'callout',
    label: 'Callout / notice',
    hint: 'A highlighted info, success, warning or danger panel.',
    icon: InfoIcon,
    fields: [
      { key: 'tone', label: 'Tone', kind: 'variant', registry: 'callout_tone' },
      { key: 'title', label: 'Title', kind: 'text' },
      { key: 'body', label: 'Body', kind: 'richtext' },
    ],
  },
  {
    type: 'buttons',
    label: 'Button group',
    hint: 'One or more standalone call-to-action buttons.',
    icon: MousePointerClickIcon,
    fields: [
      { key: 'align', label: 'Alignment', kind: 'variant', registry: 'align' },
      {
        key: 'items', label: 'Buttons', kind: 'repeater', addLabel: 'Add button',
        itemFields: [
          { key: 'label', label: 'Label', kind: 'text', required: true },
          { key: 'href', label: 'Link', kind: 'url', description: 'A full path or URL, e.g. /products.' },
          {
            key: 'style', label: 'Style', kind: 'select',
            options: [
              { value: 'primary', label: 'Primary' },
              { value: 'secondary', label: 'Secondary' },
              { value: 'outline', label: 'Outline' },
            ],
          },
          { key: 'goalEvent', label: 'A/B goal event', kind: 'text', description: 'When the page is A/B tested, a click fires this event.' },
        ],
      },
    ],
  },
  {
    type: 'media_text',
    label: 'Media + text',
    hint: 'An image beside rich text with an optional button.',
    icon: GalleryThumbnailsIcon,
    fields: [
      { key: 'image', label: 'Image', kind: 'image', required: true },
      { key: 'side', label: 'Image side', kind: 'variant', registry: 'media_side' },
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'body', label: 'Body', kind: 'richtext' },
      { key: 'ctaLabel', label: 'Button label', kind: 'text' },
      { key: 'ctaHref', label: 'Button link', kind: 'link' },
      { key: 'goalEvent', label: 'A/B goal event', kind: 'text', description: 'When the page is A/B tested, a click fires this event - match it to the experiment’s goal.' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true },
    ],
  },
  {
    type: 'feature_grid',
    label: 'Feature grid',
    hint: 'A grid of feature cards with an icon, title and text.',
    icon: LayoutGridIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'cols', label: 'Columns', kind: 'variant', registry: 'cols' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true },
      {
        key: 'items', label: 'Features', kind: 'repeater', addLabel: 'Add feature',
        itemFields: [
          { key: 'image', label: 'Icon / image', kind: 'image' },
          { key: 'title', label: 'Title', kind: 'text', required: true },
          { key: 'text', label: 'Text', kind: 'textarea' },
          { key: 'href', label: 'Link', kind: 'url' },
        ],
      },
    ],
  },
  {
    type: 'logos',
    label: 'Logo cloud',
    hint: 'A strip of partner or brand logos.',
    icon: GalleryHorizontalIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'container', label: 'Section style', kind: 'variant', registry: 'container', searchable: true },
      {
        key: 'items', label: 'Logos', kind: 'repeater', addLabel: 'Add logo',
        itemFields: [
          { key: 'image', label: 'Logo', kind: 'image', required: true },
          { key: 'href', label: 'Link', kind: 'url' },
        ],
      },
    ],
  },
  {
    type: 'timeline',
    label: 'Timeline',
    hint: 'A vertical list of dated events.',
    icon: MilestoneIcon,
    fields: [
      { key: 'heading', label: 'Heading', kind: 'text' },
      { key: 'variant', label: 'Style', kind: 'variant', registry: 'timeline_style' },
      {
        key: 'items', label: 'Events', kind: 'repeater', addLabel: 'Add event',
        itemFields: [
          { key: 'label', label: 'Date / label', kind: 'text' },
          { key: 'title', label: 'Title', kind: 'text', required: true },
          { key: 'body', label: 'Body', kind: 'textarea' },
        ],
      },
    ],
  },
]

const BY_TYPE: Record<string, CmsBlockDescriptor> = Object.fromEntries(
  CMS_BLOCK_TYPES.map((b) => [b.type, b]),
)

// ---- Admin-defined custom block types (cmsBlockTypes) ----
// Custom types are the escape hatch that skips the SYNC SENTINEL trio: the
// backend validates them against the block-type table and the storefront
// renders them server-side by interpolating props into the type's HTML
// template - The canvas registers them at runtime so palette + inspector
// treat them like builtins

// Field kinds whose values interpolate cleanly into an HTML template
export const CUSTOM_FIELD_KINDS = ['text', 'textarea', 'url', 'number', 'select'] as const

// parseCustomFields turns the block type's stored `fields` JSON into editor
// descriptors, dropping anything that isn't a template-friendly kind
export function parseCustomFields(raw: unknown): CmsBlockField[] {
  let val = raw
  if (typeof val === 'string') {
    try {
      val = JSON.parse(val)
    } catch {
      return []
    }
  }
  if (!Array.isArray(val)) return []
  const out: CmsBlockField[] = []
  for (const f of val) {
    if (typeof f !== 'object' || f === null) continue
    const o = f as Record<string, unknown>
    if (typeof o.key !== 'string' || !o.key) continue
    const kind = (CUSTOM_FIELD_KINDS as readonly string[]).includes(o.kind as string)
      ? (o.kind as CmsFieldKind)
      : 'text'
    out.push({
      key: o.key,
      label: typeof o.label === 'string' && o.label ? o.label : o.key,
      kind,
      required: o.required === true,
      placeholder: typeof o.placeholder === 'string' ? o.placeholder : undefined,
      description: typeof o.description === 'string' ? o.description : undefined,
      options: Array.isArray(o.options)
        ? (o.options as Array<{ value?: unknown; label?: unknown }>)
            .filter((op) => typeof op?.value === 'string')
            .map((op) => ({ value: op.value as string, label: typeof op.label === 'string' ? op.label : (op.value as string) }))
        : undefined,
    })
  }
  return out
}

const CUSTOM_HINT = 'Custom block - rendered from its HTML template.'

// registerCustomBlockTypes merges the fetched custom types into the palette + descriptor lookup - Idempotent per key;
// returns true when anything changed so the caller can force a re-render
export function registerCustomBlockTypes(
  types: Array<{ key: string; name: string; fields: unknown }>,
): boolean {
  let changed = false
  for (const t of types) {
    const existing = BY_TYPE[t.key]
    // Builtins win - a custom key colliding with a builtin is ignored
    if (existing && existing.hint !== CUSTOM_HINT) continue
    const desc: CmsBlockDescriptor = {
      type: t.key,
      label: t.name || t.key,
      hint: CUSTOM_HINT,
      icon: PuzzleIcon,
      fields: parseCustomFields(t.fields),
    }
    if (!existing) {
      CMS_BLOCK_TYPES.push(desc)
      BY_TYPE[t.key] = desc
      changed = true
    } else if (existing.label !== desc.label || JSON.stringify(existing.fields) !== JSON.stringify(desc.fields)) {
      existing.label = desc.label
      existing.fields = desc.fields
      changed = true
    }
  }
  return changed
}

// Block categories - used only to group the Add-block picker into scannable sections - Order here is the display order; a type with no
// mapping falls into 'Advanced' - Purely cosmetic, so it lives outside the descriptor + SYNC SENTINEL trio
export type BlockCategory = 'Layout' | 'Content' | 'Media' | 'Catalog & blog' | 'Advanced'

export const BLOCK_CATEGORY_ORDER: BlockCategory[] = [
  'Layout',
  'Content',
  'Media',
  'Catalog & blog',
  'Advanced',
]

const BLOCK_CATEGORY: Record<string, BlockCategory> = {
  section: 'Layout',
  columns: 'Layout',
  divider: 'Layout',
  hero: 'Content',
  rich_text: 'Content',
  cta_banner: 'Content',
  accordion: 'Content',
  tabs: 'Content',
  video: 'Media',
  gallery: 'Media',
  product_grid: 'Catalog & blog',
  collection_grid: 'Catalog & blog',
  category_grid: 'Catalog & blog',
  blog_grid: 'Catalog & blog',
  value_strip: 'Catalog & blog',
  promo_mosaic: 'Catalog & blog',
  html: 'Advanced',
  ref: 'Advanced',
  form: 'Advanced',
  quote: 'Content',
  stats: 'Content',
  callout: 'Content',
  buttons: 'Content',
  media_text: 'Content',
  feature_grid: 'Content',
  timeline: 'Content',
  logos: 'Media',
}

export function blockCategory(type: string): BlockCategory {
  return BLOCK_CATEGORY[type] ?? 'Advanced'
}

// Pickable "looks", grouped into named registries (a variant field names which
// one it draws from via `field.registry`) - Keep the ids in lockstep with the
// storefront registries (apps/channels/vetrina-demo/app/components/cms/variants.tsx) - Add a
// look by appending here + registering the matching component on the storefront
// - no core edit - The first entry in a registry is its default
export type BlockVariantDef = { id: string; label: string; description?: string }

export const BLOCK_REGISTRIES: Record<string, BlockVariantDef[]> = {
  // layout + container are ENTITY-AGNOSTIC and shared by every grid block (product / collection / category); the *_card
  // registries are per-entity Option ids here must match the storefront's variants.tsx registrations
  layout: [
    { id: 'grid', label: 'Grid', description: 'Responsive card grid' },
    { id: 'featured', label: 'Featured', description: 'First item enlarged, rest in a grid' },
    { id: 'carousel', label: 'Carousel', description: 'Horizontal scrolling rail' },
  ],
  product_card: [
    { id: 'standard', label: 'Standard', description: 'Full card with details' },
    { id: 'compact', label: 'Compact', description: 'Smaller, denser card' },
    { id: 'overlay', label: 'Overlay', description: 'Image with title overlaid' },
  ],
  collection_card: [
    { id: 'standard', label: 'Standard', description: 'Glass tile with product count' },
    { id: 'compact', label: 'Compact', description: 'Dense row with product count' },
  ],
  category_card: [
    { id: 'standard', label: 'Standard', description: 'Square tile with product count' },
    { id: 'compact', label: 'Compact', description: 'Dense row with product count' },
    { id: 'circle', label: 'Circle', description: 'Round icon chip, department-row style' },
  ],
  container: [
    { id: 'default', label: 'Bordered', description: 'Bordered band + “view all”' },
    { id: 'plain', label: 'Plain', description: 'No band, simple heading' },
    { id: 'muted', label: 'Muted', description: 'Shaded band, centered heading' },
  ],
  // ---------- second-pack content-block variants ----------
  quote_style: [
    { id: 'card', label: 'Card', description: 'Bordered glass panel' },
    { id: 'large', label: 'Large', description: 'Big centered pull-quote' },
    { id: 'minimal', label: 'Minimal', description: 'Left accent bar, no frame' },
  ],
  callout_tone: [
    { id: 'info', label: 'Info', description: 'Blue informational panel' },
    { id: 'success', label: 'Success', description: 'Green confirmation panel' },
    { id: 'warning', label: 'Warning', description: 'Amber caution panel' },
    { id: 'danger', label: 'Danger', description: 'Red alert panel' },
  ],
  align: [
    { id: 'left', label: 'Left', description: 'Align to the start' },
    { id: 'center', label: 'Center', description: 'Centered' },
    { id: 'right', label: 'Right', description: 'Align to the end' },
  ],
  cols: [
    { id: 'two', label: '2 columns', description: 'Two per row' },
    { id: 'three', label: '3 columns', description: 'Three per row' },
    { id: 'four', label: '4 columns', description: 'Four per row' },
  ],
  media_side: [
    { id: 'left', label: 'Image left', description: 'Image on the left, text right' },
    { id: 'right', label: 'Image right', description: 'Image on the right, text left' },
  ],
  timeline_style: [
    { id: 'vertical', label: 'Vertical', description: 'Single left rail' },
    { id: 'alternating', label: 'Alternating', description: 'Items zig-zag left/right' },
  ],
}

export function registryOptions(name: string | undefined): BlockVariantDef[] {
  return name ? (BLOCK_REGISTRIES[name] ?? []) : []
}

export function blockDescriptor(type: string): CmsBlockDescriptor | undefined {
  return BY_TYPE[type]
}

export function blockLabel(type: string): string {
  return BY_TYPE[type]?.label ?? type
}

// genId mints a stable per-block id - randomId carries the secure-context guard
export function genId(): string {
  return `blk_${randomId()}`
}

// newBlock builds a block of the given type, seeding required selects with
// their first option so a freshly-added block is already valid
export function newBlock(type: string): CmsBlock {
  const props: Record<string, unknown> = {}
  for (const f of blockDescriptor(type)?.fields ?? []) {
    if (f.kind === 'select' && f.required && f.options?.length) {
      props[f.key] = f.options[0].value
    }
  }
  return { id: genId(), type, props }
}

// --- prop accessors (tolerant; props is an untyped JSON bag) ---------------

export function readString(props: Record<string, unknown>, key: string): string {
  const v = props[key]
  return typeof v === 'string' ? v : ''
}

export function readNumber(props: Record<string, unknown>, key: string): number | undefined {
  const v = props[key]
  if (typeof v === 'number' && Number.isFinite(v)) return v
  return undefined
}

// --- document (de)serialization -------------------------------------------

function isCmsBlock(value: unknown): value is CmsBlock {
  if (!value || typeof value !== 'object') return false
  const b = value as Record<string, unknown>
  return typeof b.id === 'string' && typeof b.type === 'string'
}

// parseDocument decodes a page body (the JSON `body` scalar) into a block
// Document, or null when the body is absent or freeform - matching
// blockkit.ParseDocument's ok=false branch and the storefront parseDocument
// A block document is identified by the presence of a `blocks` array
export function parseDocument(body: unknown): CmsDocument | null {
  if (!body || typeof body !== 'object' || Array.isArray(body)) return null
  const obj = body as Record<string, unknown>
  if (!Array.isArray(obj.blocks)) return null
  const blocks = obj.blocks.filter(isCmsBlock).map((b) => ({
    id: b.id,
    type: b.type,
    props: (b.props && typeof b.props === 'object' ? b.props : {}) as Record<string, unknown>,
    ...(Array.isArray(b.blocks) ? { blocks: b.blocks } : {}),
  })) as CmsBlock[]
  const version = typeof obj.version === 'number' ? obj.version : CMS_DOCUMENT_VERSION
  return { version, blocks }
}

// serializeDocument builds the persisted body from the editor's block list
export function serializeDocument(blocks: CmsBlock[]): CmsDocument {
  return { version: CMS_DOCUMENT_VERSION, blocks }
}

// shortPreview renders a one-line summary of a block for a collapsed row
export function shortPreview(b: CmsBlock): string {
  const heading = readString(b.props, 'heading')
  switch (b.type) {
    case 'hero':
    case 'cta_banner':
      return heading || '(no heading)'
    case 'html':
      return readString(b.props, 'html').replace(/\s+/g, ' ').slice(0, 60) || '(empty)'
    case 'product_grid': {
      const src = readString(b.props, 'source') || 'newest'
      return heading ? `${heading} · ${src}` : src
    }
    case 'collection_grid':
    case 'category_grid':
    case 'blog_grid':
      return heading || blockLabel(b.type)
    case 'section':
    case 'columns': {
      const n = Array.isArray(b.blocks) ? b.blocks.length : 0
      return `${n} block${n === 1 ? '' : 's'}`
    }
    case 'promo_mosaic':
      return 'Auto merchandising tiles'
    case 'accordion':
    case 'tabs':
    case 'gallery':
    case 'stats':
    case 'buttons':
    case 'feature_grid':
    case 'logos':
    case 'value_strip':
    case 'timeline': {
      const n = Array.isArray(b.props.items) ? (b.props.items as unknown[]).length : 0
      return heading ? `${heading} · ${n}` : `${n} item${n === 1 ? '' : 's'}`
    }
    case 'quote':
      return readString(b.props, 'quote').replace(/\s+/g, ' ').slice(0, 60) || '(empty)'
    case 'callout':
      return readString(b.props, 'title') || readString(b.props, 'tone') || 'Callout'
    case 'media_text':
      return heading || 'Media + text'
    case 'video':
      return readString(b.props, 'url') || '(no video)'
    case 'rich_text':
      return 'Rich text'
    case 'ref':
      return readString(b.props, 'slug') ? `→ ${readString(b.props, 'slug')}` : '(no section)'
    case 'divider':
      return readString(b.props, 'style') === 'blank' ? 'Spacer' : 'Divider'
    default:
      return heading
  }
}
