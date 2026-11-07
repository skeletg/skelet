import type { AssistantShownItem } from '@/graphql/schema.types'

// What a reply showed, as the backend reads it from the recap it wrote for the model
export type Shown =
  | { kind: 'card'; name: string; id: string }
  | { kind: 'panel'; name: string; id: string }
  | { kind: 'button'; label: string; path: string }
  | { kind: 'moved'; path: string }
  | { kind: 'cart' }
  | { kind: 'text'; text: string }

type Item = Pick<AssistantShownItem, 'kind' | 'productId' | 'name' | 'label' | 'path' | 'text'>

export function shownFrom(items: readonly Item[]): Shown[] {
  return items.flatMap((s): Shown[] => {
    switch (s.kind) {
      case 'CARD':
      case 'PANEL':
        return s.productId ? [{ kind: s.kind === 'CARD' ? 'card' : 'panel', name: s.name ?? 'A product', id: s.productId }] : []
      case 'BUTTON':
        return [{ kind: 'button', label: s.label ?? '', path: s.path ?? '' }]
      case 'MOVED':
        return [{ kind: 'moved', path: s.path ?? '' }]
      case 'CART':
        return [{ kind: 'cart' }]
      default:
        return s.text ? [{ kind: 'text', text: s.text }] : []
    }
  })
}

const NEXT = /\[\[next:([\s\S]*?)\]\]\s*$/

// A round's words without the suggestions line the model ends on, which reach the shopper as buttons rather than words
export function wordsOf(text: string): string {
  return text.replace(NEXT, '').trim()
}

// No words and nothing shown means the storefront put up its apology instead of an answer
export function unanswered(turn: { reply: string; recap: string; incomplete: boolean }): boolean {
  return !turn.incomplete && !turn.reply.trim() && !turn.recap.trim()
}

const LABELS: Record<string, string> = {
  get_store_info: 'Read the store details',
  search_products: 'Searched the catalog',
  get_product: 'Looked up a product',
  get_deals: 'Checked the deals',
  list_categories: 'Browsed the categories',
  search_store_pages: 'Searched the store pages',
  estimate_shipping: 'Priced delivery',
  get_cart: 'Read the cart',
  update_cart: 'Changed the cart',
  start_checkout: 'Sent to checkout',
  show_products: 'Showed product cards',
  show_product_details: 'Showed a product panel',
  show_results: 'Linked a product list',
  show_page: 'Linked a page',
  get_current_page: 'Read the page the shopper is on',
  update_listing: 'Refiltered the list',
  find_similar_products: 'Looked for similar products',
  get_my_orders: "Read the shopper's orders",
  get_my_order: 'Looked up an order',
  start_return: 'Started a return',
}

// A tool added since gets a readable name rather than its own
export function toolLabel(tool: string): string {
  const words = tool.replaceAll('_', ' ')
  return LABELS[tool] ?? words.charAt(0).toUpperCase() + words.slice(1)
}

function brief(v: unknown): string {
  if (typeof v === 'string') return v.length > 40 ? `"${v.slice(0, 40)}…"` : `"${v}"`
  if (Array.isArray(v)) return v.length <= 3 && v.every((x) => typeof x !== 'object') ? v.join(', ') : `${v.length} items`
  if (v && typeof v === 'object') {
    return Object.entries(v)
      .map(([k, x]) => `${k}=${Array.isArray(x) ? x.join('/') : String(x)}`)
      .join(', ')
  }
  return String(v)
}

export function argChips(input: string, most = 5): string[] {
  let parsed: unknown
  try {
    parsed = JSON.parse(input)
  } catch {
    return []
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return []
  const chips = Object.entries(parsed)
    .filter(([, v]) => v !== null && v !== '' && !(Array.isArray(v) && v.length === 0))
    .map(([k, v]) => `${k}: ${brief(v)}`)
    .map((c) => (c.length > 60 ? `${c.slice(0, 59)}…` : c))
  return chips.length > most ? [...chips.slice(0, most), `+${chips.length - most} more`] : chips
}

export function firstLine(text: string, most = 90): string {
  const line = text.trim().split('\n')[0] ?? ''
  return line.length > most ? `${line.slice(0, most)}…` : line
}
