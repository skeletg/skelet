import type { ReactNode } from 'react'

// Generic UI slot registry - a registry per surface (nav, detail sections, list extensions, lexical blocks, CMS blocks) stopped paying at five
// Zone is free-form, conventionally '<surface>:<target>' so a new surface needs no core change - today `list-toolbar:<target>` ctx {table}, `home:widgets` no ctx

export type SlotEntry<TCtx = unknown> = {
  id: string // conventionally '<plugin-name>:thing' - duplicates throw at registration
  zone: string
  order?: number // lower renders earlier, default 100 - core sits at 10-50
  render: (ctx: TCtx) => ReactNode
}

const slots: SlotEntry[] = []

export function registerSlot<TCtx = unknown>(entry: SlotEntry<TCtx>) {
  if (slots.some((s) => s.id === entry.id && s.zone === entry.zone)) {
    throw new Error(`slots: duplicate id "${entry.id}" in zone "${entry.zone}"`)
  }
  slots.push(entry as SlotEntry)
}

export function getSlots<TCtx = unknown>(zone: string | undefined): SlotEntry<TCtx>[] {
  if (!zone) return []
  return slots
    .filter((s) => s.zone === zone)
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100)) as SlotEntry<TCtx>[]
}
