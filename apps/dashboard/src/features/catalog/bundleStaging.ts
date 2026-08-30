// Staging layer for the Bundle structure editor
//
// Edits are modelled as discrete ops applied to a derived view, so the UI shows staged state and the
// page commits through the product page's global "Save changes (N)" registry rather than its own mutations
// Ops carry tempIds for entities created this session, resolved to real ids as adds complete

import type { BundleInputType } from '@/graphql/schema.types'
import { collapseOps, isTemp, type OpView } from '@/features/catalog/stagingOps'
import type { SectionDiffEntry } from './productSaveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'

export type SelectionProduct = {
  id: string
  sku?: string | null
  name?: string | null
  kind: string
  defaultPrice: number
  defaultCurrencyCode: string
}

export type Selection = {
  id: string
  product: SelectionProduct
  quantity: number
  isDefault: boolean
  sortOrder: number
}

export type OptionContent = { locale: string; title: string }

export type BundleOption = {
  id: string
  inputType: BundleInputType
  isRequired: boolean
  sortOrder: number
  title: string
  content: OptionContent[]
  selections: Selection[]
}

export { TEMP_PREFIX, isTemp, newTempId } from '@/features/catalog/stagingOps'

// ─── Op union ───────────────────────────────────────────────────────

export type AddOptionOp = {
  kind: 'add-option'
  tempId: string
  title: string
  inputType: BundleInputType
  isRequired: boolean
  sortOrder: number
  locale: string
}

export type UpdateOptionOp = {
  kind: 'update-option'
  id: string // tempId or real
  // All optional - the coalescer merges several updates on one option into a single op
  title?: string
  titleLocale?: string
  inputType?: BundleInputType
  isRequired?: boolean
  sortOrder?: number
}

export type DeleteOptionOp = {
  kind: 'delete-option'
  id: string
}

export type AddSelectionOp = {
  kind: 'add-selection'
  tempId: string
  optionRef: string // tempId or real id of the parent option
  product: SelectionProduct
  quantity: number
  isDefault: boolean
  sortOrder: number
}

export type UpdateSelectionOp = {
  kind: 'update-selection'
  id: string
  quantity?: number
  isDefault?: boolean
  sortOrder?: number
}

export type DeleteSelectionOp = {
  kind: 'delete-selection'
  id: string
}

export type PendingOp =
  | AddOptionOp
  | UpdateOptionOp
  | DeleteOptionOp
  | AddSelectionOp
  | UpdateSelectionOp
  | DeleteSelectionOp

// ─── Coalescing ─────────────────────────────────────────────────────

// Simplifies the queue so the UI's diff count matches what the user perceives as changes -
// without it, "edit a title then change it back" still reports a pending change
//
//   add(tempId) -> delete(tempId)   drop both
//   add(tempId) -> update(tempId)   merge fields into the add
//   update(id)  -> update(id)       merge fields into the first update
//   delete(id) after update(id)     drop the updates, keep the delete
//   update(id) after delete(id)     drop the update, delete wins
//
// Order-preserving - the survivor stays at its earliest position
// What the shared collapser needs to know about a bundle op - see stagingOps. `entity` is what keeps
// an option update from merging into a selection update that happens to carry the same id
const view = (op: PendingOp): OpView => {
  const [role, entity] = op.kind.split('-') as ['add' | 'update' | 'delete', string]
  return { role, entity, id: op.kind === 'add-option' || op.kind === 'add-selection' ? op.tempId : op.id }
}

export function reduceOps(ops: PendingOp[]): PendingOp[] {
  // Drop add+delete pairs by tempId
  let result: PendingOp[] = [...ops]
  result = result.filter((op, idx) => {
    if (op.kind !== 'add-option' && op.kind !== 'add-selection') return true
    const tempId = op.tempId
    return !result.some(
      (o, i) =>
        i > idx &&
        ((o.kind === 'delete-option' && op.kind === 'add-option' && o.id === tempId) ||
          (o.kind === 'delete-selection' && op.kind === 'add-selection' && o.id === tempId)),
    )
  })
  // Drop the delete ops that paired off above
  result = result.filter((op) => {
    if (op.kind !== 'delete-option' && op.kind !== 'delete-selection') return true
    const id = op.id
    if (!isTemp(id)) return true
    // Orphaned delete - keep it; the committer skips temp deletes anyway
    return !ops.some((o) => {
      if (op.kind === 'delete-option' && o.kind === 'add-option') return o.tempId === id
      if (op.kind === 'delete-selection' && o.kind === 'add-selection') return o.tempId === id
      return false
    })
  })

  return collapseOps(result, view, {
    updates: (acc, next) =>
      ({ ...acc, ...next, id: (acc as UpdateOptionOp | UpdateSelectionOp).id }) as PendingOp,
    intoAdd: (add, upd) =>
      mergeAddWithUpdate(add as AddOptionOp | AddSelectionOp, upd as UpdateOptionOp | UpdateSelectionOp),
  })
}

function mergeAddWithUpdate(add: AddOptionOp | AddSelectionOp, upd: UpdateOptionOp | UpdateSelectionOp): PendingOp {
  if (add.kind === 'add-option' && upd.kind === 'update-option') {
    return {
      ...add,
      title: upd.title ?? add.title,
      inputType: upd.inputType ?? add.inputType,
      isRequired: upd.isRequired ?? add.isRequired,
      sortOrder: upd.sortOrder ?? add.sortOrder,
    }
  }
  if (add.kind === 'add-selection' && upd.kind === 'update-selection') {
    return {
      ...add,
      quantity: upd.quantity ?? add.quantity,
      isDefault: upd.isDefault ?? add.isDefault,
      sortOrder: upd.sortOrder ?? add.sortOrder,
    }
  }
  // Type checker convinces itself we covered every case above
  return add
}

// ─── Apply ──────────────────────────────────────────────────────────

// A derived view with every pending op materialised - the UI renders from this, so a staged
// "Add option" appears immediately even though no mutation has run
export function applyOps(original: readonly BundleOption[], ops: readonly PendingOp[]): BundleOption[] {
  let opts: BundleOption[] = original.map((o) => ({
    ...o,
    content: [...o.content],
    selections: o.selections.map((s) => ({ ...s })),
  }))

  for (const op of ops) {
    switch (op.kind) {
      case 'add-option':
        opts.push({
          id: op.tempId,
          title: op.title,
          inputType: op.inputType,
          isRequired: op.isRequired,
          sortOrder: op.sortOrder,
          content: [{ locale: op.locale, title: op.title }],
          selections: [],
        })
        break
      case 'update-option':
        opts = opts.map((o) => {
          if (o.id !== op.id) return o
          const next = { ...o }
          if (op.title !== undefined) next.title = op.title
          if (op.inputType !== undefined) next.inputType = op.inputType
          if (op.isRequired !== undefined) next.isRequired = op.isRequired
          if (op.sortOrder !== undefined) next.sortOrder = op.sortOrder
          if (op.title !== undefined && op.titleLocale) {
            const locale = op.titleLocale
            const existing = next.content.findIndex((c) => c.locale === locale)
            if (existing >= 0) {
              next.content = next.content.map((c, i) =>
                i === existing ? { ...c, title: op.title! } : c,
              )
            } else {
              next.content = [...next.content, { locale, title: op.title }]
            }
          }
          return next
        })
        break
      case 'delete-option':
        opts = opts.filter((o) => o.id !== op.id)
        break
      case 'add-selection':
        opts = opts.map((o) => {
          if (o.id !== op.optionRef) return o
          return {
            ...o,
            selections: [
              ...o.selections,
              {
                id: op.tempId,
                product: op.product,
                quantity: op.quantity,
                isDefault: op.isDefault,
                sortOrder: op.sortOrder,
              },
            ],
          }
        })
        break
      case 'update-selection':
        opts = opts.map((o) => ({
          ...o,
          selections: o.selections.map((s) => {
            if (s.id !== op.id) return s
            const next = { ...s }
            if (op.quantity !== undefined) next.quantity = op.quantity
            if (op.isDefault !== undefined) next.isDefault = op.isDefault
            if (op.sortOrder !== undefined) next.sortOrder = op.sortOrder
            return next
          }),
        }))
        break
      case 'delete-selection':
        opts = opts.map((o) => ({
          ...o,
          selections: o.selections.filter((s) => s.id !== op.id),
        }))
        break
    }
  }
  return opts
}

// ─── Diff for the save-registry ─────────────────────────────────────

// One row per surviving op, with before/after strings in plain English for SaveConfirmDialog
export function opsToDiff(original: readonly BundleOption[], ops: readonly PendingOp[]): SectionDiffEntry[] {
  const byId = new Map(original.map((o) => [o.id, o]))
  const selById = new Map<string, { option: BundleOption; selection: Selection }>()
  for (const o of original) {
    for (const s of o.selections) selById.set(s.id, { option: o, selection: s })
  }

  return ops.flatMap((op): SectionDiffEntry[] => {
    switch (op.kind) {
      case 'add-option':
        return [added(`Option · ${op.title}`, `${op.inputType.toLowerCase()}${op.isRequired ? ' · required' : ''}`)]
      case 'update-option': {
        const prev = byId.get(op.id)
        const label = `Option · ${prev?.title ?? op.id}`
        const rows: SectionDiffEntry[] = []
        const push = (field: string, before: unknown, after: unknown) => {
          const e = changed(`${label} · ${field}`, before, after)
          if (e) rows.push(e)
        }
        if (op.title !== undefined) push('title', prev?.title, op.title)
        if (op.inputType !== undefined) push('style', prev?.inputType.toLowerCase(), op.inputType.toLowerCase())
        if (op.isRequired !== undefined) push('required', prev?.isRequired, op.isRequired)
        if (op.sortOrder !== undefined) push('order', prev?.sortOrder, op.sortOrder)
        return rows
      }
      case 'delete-option': {
        const prev = byId.get(op.id)
        return [removed(`Option · ${prev?.title ?? op.id}`, prev ? `${prev.selections.length} selection(s)` : undefined)]
      }
      case 'add-selection':
        return [added(`Selection · ${op.product.name ?? op.product.sku ?? ''}`, `qty ${op.quantity}${op.isDefault ? ' · default' : ''}`)]
      case 'update-selection': {
        const prev = selById.get(op.id)
        const label = `Selection · ${prev?.selection.product.name ?? prev?.selection.product.sku ?? op.id}`
        const rows: SectionDiffEntry[] = []
        const push = (field: string, before: unknown, after: unknown) => {
          const e = changed(`${label} · ${field}`, before, after)
          if (e) rows.push(e)
        }
        if (op.quantity !== undefined) push('qty', prev?.selection.quantity, op.quantity)
        if (op.isDefault !== undefined) push('default', prev?.selection.isDefault, op.isDefault)
        if (op.sortOrder !== undefined) push('order', prev?.selection.sortOrder, op.sortOrder)
        return rows
      }
      case 'delete-selection': {
        const prev = selById.get(op.id)
        return [removed(`Selection · ${prev?.selection.product.name ?? prev?.selection.product.sku ?? op.id}`)]
      }
    }
  })
}

// ─── Commit ─────────────────────────────────────────────────────────

// The four mutation entry points the parent threads in - the id-bearing returns are what let
// the next op resolve a tempId to a real one before it fires
export type Committers = {
  addOption: (op: AddOptionOp & { productId: string }) => Promise<string>
  updateOption: (op: UpdateOptionOp) => Promise<void>
  upsertOptionContent: (op: { optionId: string; locale: string; title: string }) => Promise<void>
  deleteOption: (op: { id: string }) => Promise<void>
  addSelection: (op: AddSelectionOp) => Promise<string>
  updateSelection: (op: UpdateSelectionOp) => Promise<void>
  deleteSelection: (op: { id: string }) => Promise<void>
}

// Builds the tempId -> realId map incrementally, so a later op referencing an added entity writes to the right row - An error aborts the
// loop and propagates to the parent's global save handler - ops that already succeeded are NOT rolled back, they're dropped before retry
export async function commitOps(
  ops: readonly PendingOp[],
  productId: string,
  committers: Committers,
): Promise<{ idMap: Map<string, string> }> {
  const idMap = new Map<string, string>()
  const resolve = (id: string) => idMap.get(id) ?? id

  for (const op of ops) {
    switch (op.kind) {
      case 'add-option': {
        const realId = await committers.addOption({ ...op, productId })
        idMap.set(op.tempId, realId)
        break
      }
      case 'update-option': {
        const id = resolve(op.id)
        await committers.updateOption({ ...op, id })
        if (op.title !== undefined && op.titleLocale) {
          await committers.upsertOptionContent({
            optionId: id,
            locale: op.titleLocale,
            title: op.title,
          })
        }
        break
      }
      case 'delete-option': {
        const id = resolve(op.id)
        if (isTemp(id)) break // never made it to the server
        await committers.deleteOption({ id })
        break
      }
      case 'add-selection': {
        const optionId = resolve(op.optionRef)
        const realId = await committers.addSelection({ ...op, optionRef: optionId })
        idMap.set(op.tempId, realId)
        break
      }
      case 'update-selection': {
        const id = resolve(op.id)
        await committers.updateSelection({ ...op, id })
        break
      }
      case 'delete-selection': {
        const id = resolve(op.id)
        if (isTemp(id)) break
        await committers.deleteSelection({ id })
        break
      }
    }
  }

  return { idMap }
}
