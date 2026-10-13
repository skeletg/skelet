// Staging layer for the Product Options editor
//
// Custom options are per-product customer-facing inputs (text, select, date, file) - each edit
// is a discrete op applied to a derived view, so the UI shows staged state and the section
// commits on page-level save
// Same shape as bundleStaging.ts (reduce/apply/diff/commit, tempId resolution) but with two
// entity types - options AND values, values belonging to a parent option

import type { ProductOptionInputType, ProductOptionPriceType } from '@/graphql/schema.types'
import { added, removed } from '@/shared/saving/diff'
import { collapseOps, isTemp, type OpView } from '@/features/catalog/stagingOps'
import type { SectionDiffEntry } from './productSaveRegistry'

export type SourceProduct = {
  id: string
  sku?: string | null
  name?: string | null
  kind: string
  defaultPrice: number
  defaultCurrencyCode: string
}

// Per-locale content rows. `title`/`helpText` on the parent are the values resolved to the dashboard's request locale;
// these carry every locale's copy so the editor can show/edit any of them
export type ValueContent = { locale: string; title: string }
export type OptionContent = { locale: string; title: string; helpText: string }

// Visibility condition shapes (wire format of ProductOption.visibleIf - snake_case keys, matching the backend's JSONB)
// - Rules may only reference options with strictly LOWER sortOrder; the backend rejects violations
export type VisibilityRule = {
  option_id: string
  op: 'anyOf' | 'noneOf' | 'filled' | 'empty'
  value_ids?: string[]
}

export type VisibilityGroup = {
  logic?: 'and' | 'or'
  rules?: VisibilityRule[]
  groups?: VisibilityGroup[]
}

// The JSON scalar arrives as `unknown` from codegen - narrow it at the
// boundary - Malformed shapes read as "no condition"
export function parseVisibleIfGroup(raw: unknown): VisibilityGroup | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const g = raw as VisibilityGroup
  if (!Array.isArray(g.rules) && !Array.isArray(g.groups)) return null
  return g
}

// A FIXED surcharge set for one channel - without one, the base amount applies only on channels selling in the
// product's own currency, and the choice can't be bought anywhere else
export type ChannelPrice = { channelId: string; priceValue: number }
export type ChannelPriceEdits = Record<string, number | null>

export function mergeChannelEdits(
  a: ChannelPriceEdits | undefined,
  b: ChannelPriceEdits | undefined,
): ChannelPriceEdits | undefined {
  if (!a) return b
  if (!b) return a
  return { ...a, ...b }
}

export function applyChannelEdits(prices: readonly ChannelPrice[] | undefined, edits: ChannelPriceEdits | undefined): ChannelPrice[] {
  const out = (prices ?? []).filter((p) => !edits || !(p.channelId in edits))
  for (const [channelId, priceValue] of Object.entries(edits ?? {})) {
    if (priceValue !== null) out.push({ channelId, priceValue })
  }
  return out
}

export type OptionValue = {
  id: string
  sortOrder: number
  priceValue: number
  priceType: ProductOptionPriceType
  isDefault: boolean
  quantity: number
  // Picking this value forces + locks the cart line quantity - Null/absent
  // = no constraint
  forcedLineQty?: number | null
  title: string
  content?: ValueContent[]
  // Optional linked SIMPLE product - When set the value's effective
  // cart contribution is (sourceProduct.defaultPrice * quantity) + priceValue
  sourceProduct?: SourceProduct | null
  channelPrices?: ChannelPrice[]
}

export type ProductOption = {
  id: string
  inputType: ProductOptionInputType
  isRequired: boolean
  sortOrder: number
  title: string
  helpText: string
  content?: OptionContent[]
  priceValue?: number | null
  priceType?: ProductOptionPriceType | null
  maxChars?: number | null
  maxFileBytes?: number | null
  fileExtensions?: string | null
  // Raw JSON scalar from codegen - narrow with parseVisibleIfGroup()
  visibleIf?: unknown
  channelPrices?: ChannelPrice[]
  values: OptionValue[]
}

// contentTitleFor / contentHelpFor resolve the copy for a specific locale from the per-locale content rows, falling
// back to the request-locale resolved scalar when that locale has no row yet
export function optionContentFor(o: ProductOption, locale: string): { title: string; helpText: string } {
  const row = o.content?.find((c) => c.locale === locale)
  return { title: row?.title ?? o.title, helpText: row?.helpText ?? o.helpText }
}

export function valueContentFor(v: OptionValue, locale: string): string {
  return v.content?.find((c) => c.locale === locale)?.title ?? v.title
}

// upsertOptionContentRow returns a new content array with `locale`'s row merged in (created if absent), seeding missing
// fields from the option's resolved scalars - Used by applyOps to keep the staged view coherent
function upsertOptionContentRow(
  o: ProductOption,
  locale: string,
  title: string | undefined,
  helpText: string | undefined,
): OptionContent[] {
  const content = [...(o.content ?? [])]
  const i = content.findIndex((c) => c.locale === locale)
  const base = i >= 0 ? content[i] : { locale, title: o.title, helpText: o.helpText }
  const merged: OptionContent = {
    locale,
    title: title ?? base.title,
    helpText: helpText ?? base.helpText,
  }
  if (i >= 0) content[i] = merged
  else content.push(merged)
  return content
}

function upsertValueContentRow(v: OptionValue, locale: string, title: string): ValueContent[] {
  const content = [...(v.content ?? [])]
  const i = content.findIndex((c) => c.locale === locale)
  if (i >= 0) content[i] = { locale, title }
  else content.push({ locale, title })
  return content
}

export { TEMP_PREFIX, isTemp, newTempId } from '@/features/catalog/stagingOps'

// ─── Op union ───────────────────────────────────────────────────────

export type AddOptionOp = {
  kind: 'add-option'
  tempId: string
  inputType: ProductOptionInputType
  isRequired: boolean
  sortOrder: number
  title: string
  helpText: string
  locale: string
  // Non-select inputs only:
  priceValue: number | null
  priceType: ProductOptionPriceType | null
  // Text-input only:
  maxChars: number | null
  // File-input only:
  maxFileBytes: number | null
  fileExtensions: string | null
  // Carried when an update folds into this add - null/absent = always visible
  visibleIf?: VisibilityGroup | null
  channelPrices?: ChannelPriceEdits
}

export type UpdateOptionOp = {
  kind: 'update-option'
  id: string
  // Fields are optional - coalescer merges multiple updates on the same option
  title?: string
  helpText?: string
  titleLocale?: string
  inputType?: ProductOptionInputType
  isRequired?: boolean
  sortOrder?: number
  priceValue?: number | null
  priceType?: ProductOptionPriceType | null
  maxChars?: number | null
  maxFileBytes?: number | null
  fileExtensions?: string | null
  // null = clear the condition (always visible) - Undefined = no change
  visibleIf?: VisibilityGroup | null
  channelPrices?: ChannelPriceEdits
}

export type DeleteOptionOp = {
  kind: 'delete-option'
  id: string
}

export type AddValueOp = {
  kind: 'add-value'
  tempId: string
  optionRef: string
  sortOrder: number
  priceValue: number
  priceType: ProductOptionPriceType
  isDefault: boolean
  quantity: number
  forcedLineQty?: number | null
  title: string
  locale: string
  // Optional product link - If set, sourceProductId carries the id;
  // sourceProduct is the display snapshot for the staged view
  sourceProductId?: string | null
  sourceProduct?: SourceProduct | null
  channelPrices?: ChannelPriceEdits
}

export type UpdateValueOp = {
  kind: 'update-value'
  id: string
  title?: string
  titleLocale?: string
  sortOrder?: number
  priceValue?: number
  priceType?: ProductOptionPriceType
  isDefault?: boolean
  quantity?: number
  // null = clear the forced quantity - Undefined = no change
  forcedLineQty?: number | null
  // null = unlink (clear) - Undefined = no change
  sourceProductId?: string | null
  sourceProduct?: SourceProduct | null
  channelPrices?: ChannelPriceEdits
}

export type DeleteValueOp = {
  kind: 'delete-value'
  id: string
}

export type PendingOp =
  | AddOptionOp
  | UpdateOptionOp
  | DeleteOptionOp
  | AddValueOp
  | UpdateValueOp
  | DeleteValueOp

// ─── Coalescing ─────────────────────────────────────────────────────

// What the shared collapser needs to know about an option op - see stagingOps
const view = (op: PendingOp): OpView => {
  const [role, entity] = op.kind.split('-') as ['add' | 'update' | 'delete', string]
  return { role, entity, id: op.kind === 'add-option' || op.kind === 'add-value' ? op.tempId : op.id }
}

export function reduceOps(ops: PendingOp[]): PendingOp[] {
  // A staged add later deleted cancels out - everything referencing its tempId (values under it, updates, the delete
  // itself) goes with it, or a phantom op survives and fails parseID at commit
  // Stays here rather than in stagingOps: an option cascades to the values under it, and no other
  // staging queue has anything to cascade to
  const cancelled = new Set<string>()
  ops.forEach((op, idx) => {
    if (op.kind !== 'add-option' && op.kind !== 'add-value') return
    const deleted = ops.some(
      (o, i) =>
        i > idx &&
        ((o.kind === 'delete-option' && op.kind === 'add-option' && o.id === op.tempId) ||
          (o.kind === 'delete-value' && op.kind === 'add-value' && o.id === op.tempId)),
    )
    if (deleted) cancelled.add(op.tempId)
  })
  for (const op of ops) {
    if (op.kind === 'add-value' && cancelled.has(op.optionRef)) cancelled.add(op.tempId)
  }
  const result: PendingOp[] = ops.filter((op) => {
    switch (op.kind) {
      case 'add-option':
        return !cancelled.has(op.tempId)
      case 'add-value':
        return !cancelled.has(op.tempId) && !cancelled.has(op.optionRef)
      default:
        return !cancelled.has(op.id)
    }
  })

  return collapseOps(result, view, {
    updates: (acc, next) => {
      const a = acc as UpdateOptionOp | UpdateValueOp
      const n = next as UpdateOptionOp | UpdateValueOp
      const channelPrices = mergeChannelEdits(a.channelPrices, n.channelPrices)
      return { ...a, ...n, id: a.id, ...(channelPrices ? { channelPrices } : {}) } as PendingOp
    },
    intoAdd: (add, upd) =>
      mergeAddWithUpdate(add as AddOptionOp | AddValueOp, upd as UpdateOptionOp | UpdateValueOp),
  })
}

function mergeAddWithUpdate(add: AddOptionOp | AddValueOp, upd: UpdateOptionOp | UpdateValueOp): PendingOp {
  if (add.kind === 'add-option' && upd.kind === 'update-option') {
    return {
      ...add,
      title: upd.title ?? add.title,
      helpText: upd.helpText ?? add.helpText,
      inputType: upd.inputType ?? add.inputType,
      isRequired: upd.isRequired ?? add.isRequired,
      sortOrder: upd.sortOrder ?? add.sortOrder,
      priceValue: upd.priceValue !== undefined ? upd.priceValue : add.priceValue,
      priceType: upd.priceType !== undefined ? upd.priceType : add.priceType,
      maxChars: upd.maxChars !== undefined ? upd.maxChars : add.maxChars,
      maxFileBytes: upd.maxFileBytes !== undefined ? upd.maxFileBytes : add.maxFileBytes,
      fileExtensions: upd.fileExtensions !== undefined ? upd.fileExtensions : add.fileExtensions,
      visibleIf: upd.visibleIf !== undefined ? upd.visibleIf : add.visibleIf,
      channelPrices: mergeChannelEdits(add.channelPrices, upd.channelPrices),
    }
  }
  if (add.kind === 'add-value' && upd.kind === 'update-value') {
    return {
      ...add,
      title: upd.title ?? add.title,
      sortOrder: upd.sortOrder ?? add.sortOrder,
      priceValue: upd.priceValue ?? add.priceValue,
      priceType: upd.priceType ?? add.priceType,
      isDefault: upd.isDefault ?? add.isDefault,
      quantity: upd.quantity ?? add.quantity,
      forcedLineQty: upd.forcedLineQty !== undefined ? upd.forcedLineQty : add.forcedLineQty,
      // sourceProductId on the update wins (including the "unlink" null)
      sourceProductId: upd.sourceProductId !== undefined ? upd.sourceProductId : add.sourceProductId,
      sourceProduct: upd.sourceProductId !== undefined ? upd.sourceProduct ?? null : add.sourceProduct,
      channelPrices: mergeChannelEdits(add.channelPrices, upd.channelPrices),
    }
  }
  return add
}

// ─── Apply ──────────────────────────────────────────────────────────

export function applyOps(original: readonly ProductOption[], ops: readonly PendingOp[]): ProductOption[] {
  let opts: ProductOption[] = original.map((o) => ({
    ...o,
    values: o.values.map((v) => ({ ...v })),
  }))

  for (const op of ops) {
    switch (op.kind) {
      case 'add-option':
        opts.push({
          id: op.tempId,
          inputType: op.inputType,
          isRequired: op.isRequired,
          sortOrder: op.sortOrder,
          title: op.title,
          helpText: op.helpText,
          content: [{ locale: op.locale, title: op.title, helpText: op.helpText }],
          priceValue: op.priceValue,
          priceType: op.priceType,
          maxChars: op.maxChars,
          maxFileBytes: op.maxFileBytes,
          fileExtensions: op.fileExtensions,
          visibleIf: op.visibleIf ?? null,
          channelPrices: applyChannelEdits([], op.channelPrices),
          values: [],
        })
        break
      case 'update-option':
        opts = opts.map((o) => {
          if (o.id !== op.id) return o
          const next = { ...o }
          if (op.inputType !== undefined) next.inputType = op.inputType
          if (op.isRequired !== undefined) next.isRequired = op.isRequired
          if (op.sortOrder !== undefined) next.sortOrder = op.sortOrder
          if (op.priceValue !== undefined) next.priceValue = op.priceValue
          if (op.priceType !== undefined) next.priceType = op.priceType
          if (op.maxChars !== undefined) next.maxChars = op.maxChars
          if (op.maxFileBytes !== undefined) next.maxFileBytes = op.maxFileBytes
          if (op.fileExtensions !== undefined) next.fileExtensions = op.fileExtensions
          if (op.visibleIf !== undefined) next.visibleIf = op.visibleIf
          if (op.channelPrices !== undefined) next.channelPrices = applyChannelEdits(o.channelPrices, op.channelPrices)
          // Title/help text edits land on the targeted locale's content row,
          // not the request-locale scalar - so editing a translation doesn't
          // clobber the fallback shown for other locales - The UI reads copy
          // via optionContentFor, which prefers the row over the scalar
          if (op.titleLocale && (op.title !== undefined || op.helpText !== undefined)) {
            next.content = upsertOptionContentRow(next, op.titleLocale, op.title, op.helpText)
          }
          return next
        })
        break
      case 'delete-option':
        opts = opts.filter((o) => o.id !== op.id)
        break
      case 'add-value':
        opts = opts.map((o) => {
          if (o.id !== op.optionRef) return o
          return {
            ...o,
            values: [
              ...o.values,
              {
                id: op.tempId,
                sortOrder: op.sortOrder,
                priceValue: op.priceValue,
                priceType: op.priceType,
                isDefault: op.isDefault,
                quantity: op.quantity,
                forcedLineQty: op.forcedLineQty ?? null,
                title: op.title,
                content: [{ locale: op.locale, title: op.title }],
                sourceProduct: op.sourceProduct ?? null,
                channelPrices: applyChannelEdits([], op.channelPrices),
              },
            ],
          }
        })
        break
      case 'update-value':
        opts = opts.map((o) => ({
          ...o,
          values: o.values.map((v) => {
            if (v.id !== op.id) return v
            const next = { ...v }
            if (op.sortOrder !== undefined) next.sortOrder = op.sortOrder
            if (op.priceValue !== undefined) next.priceValue = op.priceValue
            if (op.priceType !== undefined) next.priceType = op.priceType
            if (op.isDefault !== undefined) next.isDefault = op.isDefault
            if (op.quantity !== undefined) next.quantity = op.quantity
            if (op.forcedLineQty !== undefined) next.forcedLineQty = op.forcedLineQty
            if (op.channelPrices !== undefined) next.channelPrices = applyChannelEdits(v.channelPrices, op.channelPrices)
            if (op.sourceProductId !== undefined) {
              next.sourceProduct = op.sourceProductId === null ? null : op.sourceProduct ?? v.sourceProduct ?? null
            }
            // Title edit lands on the targeted locale's content row (values
            // have no help text) - UI reads via valueContentFor
            if (op.title !== undefined && op.titleLocale) {
              next.content = upsertValueContentRow(next, op.titleLocale, op.title)
            }
            return next
          }),
        }))
        break
      case 'delete-value':
        opts = opts.map((o) => ({
          ...o,
          values: o.values.filter((v) => v.id !== op.id),
        }))
        break
    }
  }
  return opts
}

// ─── Diff for the save-registry ─────────────────────────────────────

export type ChannelLabel = (channelId: string) => string

function channelPriceRows(
  label: string,
  prev: readonly ChannelPrice[] | undefined,
  edits: ChannelPriceEdits | undefined,
  channelLabel: ChannelLabel,
): SectionDiffEntry[] {
  return Object.entries(edits ?? {}).flatMap(([channelId, next]): SectionDiffEntry[] => {
    const before = prev?.find((p) => p.channelId === channelId)?.priceValue ?? null
    if (before === next) return []
    return [{
      kind: 'change',
      label: `${label} · price on ${channelLabel(channelId)}`,
      before: before === null ? '' : String(before),
      after: next === null ? '' : String(next),
    }]
  })
}

function channelPriceSummary(edits: ChannelPriceEdits | undefined, channelLabel: ChannelLabel): string {
  return Object.entries(edits ?? {})
    .filter(([, v]) => v !== null)
    .map(([channelId, v]) => ` · ${channelLabel(channelId)} ${v}`)
    .join('')
}

export function opsToDiff(
  original: readonly ProductOption[],
  ops: readonly PendingOp[],
  channelLabel: ChannelLabel = () => 'another channel',
): SectionDiffEntry[] {
  const byId = new Map(original.map((o) => [o.id, o]))
  const valById = new Map<string, { option: ProductOption; value: OptionValue }>()
  for (const o of original) {
    for (const v of o.values) valById.set(v.id, { option: o, value: v })
  }

  return ops.flatMap((op): SectionDiffEntry[] => {
    switch (op.kind) {
      case 'add-option':
        return [added(`Option · ${op.title}`, `${op.inputType.toLowerCase()}${op.isRequired ? ' · required' : ''}${channelPriceSummary(op.channelPrices, channelLabel)}`)]
      case 'update-option': {
        const prev = byId.get(op.id)
        const label = `Option · ${prev?.title ?? op.id}`
        const rows: SectionDiffEntry[] = []
        const push = (field: string, before: unknown, after: unknown) => {
          rows.push({
            kind: 'change',
            label: `${label} · ${field}`,
            before: before === undefined || before === null ? '' : String(before),
            after: after === undefined || after === null ? '' : String(after),
          })
        }
        if (op.title !== undefined) push('title', prev?.title, op.title)
        if (op.helpText !== undefined) push('help text', prev?.helpText, op.helpText)
        if (op.inputType !== undefined) push('type', prev?.inputType.toLowerCase(), op.inputType.toLowerCase())
        if (op.isRequired !== undefined) push('required', prev ? (prev.isRequired ? 'yes' : 'no') : undefined, op.isRequired ? 'yes' : 'no')
        if (op.sortOrder !== undefined) push('order', prev?.sortOrder, op.sortOrder)
        if (op.priceValue !== undefined) push('price', prev?.priceValue, op.priceValue)
        if (op.priceType !== undefined) push('price type', prev?.priceType?.toLowerCase(), op.priceType?.toLowerCase())
        if (op.maxChars !== undefined) push('max chars', prev?.maxChars, op.maxChars)
        if (op.maxFileBytes !== undefined) push('max file bytes', prev?.maxFileBytes, op.maxFileBytes)
        if (op.fileExtensions !== undefined) push('extensions', prev?.fileExtensions, op.fileExtensions)
        if (op.visibleIf !== undefined) {
          push(
            'visibility',
            prev && (parseVisibleIfGroup(prev.visibleIf)?.rules?.length ?? 0) > 0 ? 'conditional' : 'always shown',
            op.visibleIf === null ? 'always shown' : 'conditional',
          )
        }
        rows.push(...channelPriceRows(label, prev?.channelPrices, op.channelPrices, channelLabel))
        return rows
      }
      case 'delete-option': {
        const prev = byId.get(op.id)
        return [removed(`Option · ${prev?.title ?? op.id}`, prev ? `${prev.values.length} value(s)` : undefined)]
      }
      case 'add-value':
        return [added(`Value · ${op.title}`, `${op.priceType.toLowerCase()} ${op.priceValue}${channelPriceSummary(op.channelPrices, channelLabel)}${op.isDefault ? ' · default' : ''}${op.forcedLineQty ? ` · locks qty ${op.forcedLineQty}` : ''}`)]
      case 'update-value': {
        const prev = valById.get(op.id)
        const label = `Value · ${prev?.value.title ?? op.id}`
        const rows: SectionDiffEntry[] = []
        const push = (field: string, before: unknown, after: unknown) => {
          rows.push({
            kind: 'change',
            label: `${label} · ${field}`,
            before: before === undefined || before === null ? '' : String(before),
            after: after === undefined || after === null ? '' : String(after),
          })
        }
        if (op.title !== undefined) push('title', prev?.value.title, op.title)
        if (op.priceValue !== undefined) push('price', prev?.value.priceValue, op.priceValue)
        if (op.priceType !== undefined) push('type', prev?.value.priceType.toLowerCase(), op.priceType.toLowerCase())
        if (op.isDefault !== undefined) push('default', prev ? (prev.value.isDefault ? 'yes' : 'no') : undefined, op.isDefault ? 'yes' : 'no')
        if (op.sortOrder !== undefined) push('order', prev?.value.sortOrder, op.sortOrder)
        if (op.quantity !== undefined) push('quantity', prev?.value.quantity, op.quantity)
        if (op.sourceProductId !== undefined) {
          const prevSrc = prev?.value.sourceProduct
          push(
            'linked product',
            prevSrc ? prevSrc.sku ?? prevSrc.name ?? prevSrc.id : 'none',
            op.sourceProductId === null ? 'none' : op.sourceProduct?.sku ?? op.sourceProduct?.name ?? op.sourceProductId,
          )
        }
        if (op.forcedLineQty !== undefined) {
          push('line qty', prev?.value.forcedLineQty != null ? `locked at ${prev.value.forcedLineQty}` : 'free', op.forcedLineQty === null ? 'free' : `locked at ${op.forcedLineQty}`)
        }
        rows.push(...channelPriceRows(label, prev?.value.channelPrices, op.channelPrices, channelLabel))
        return rows
      }
      case 'delete-value': {
        const prev = valById.get(op.id)
        return [removed(`Value · ${prev?.value.title ?? op.id}`)]
      }
    }
  })
}

// ─── Commit ─────────────────────────────────────────────────────────

export type Committers = {
  addOption: (op: AddOptionOp & { productId: string }) => Promise<string>
  updateOption: (op: UpdateOptionOp) => Promise<void>
  upsertOptionContent: (op: { optionId: string; locale: string; title: string; helpText: string }) => Promise<void>
  deleteOption: (op: { id: string }) => Promise<void>
  addValue: (op: AddValueOp) => Promise<string>
  updateValue: (op: UpdateValueOp) => Promise<void>
  upsertValueContent: (op: { valueId: string; locale: string; title: string }) => Promise<void>
  deleteValue: (op: { id: string }) => Promise<void>
  setOptionChannelPrice: (op: { optionId: string; channelId: string; priceValue: number | null }) => Promise<void>
  setValueChannelPrice: (op: { valueId: string; channelId: string; priceValue: number | null }) => Promise<void>
}

// An add carrying channel prices commits as the add and then a prices-only update of the same temp id, so a failed
// price write leaves a retry that resumes at the prices instead of adding the row twice
function splitChannelPrices(ops: readonly PendingOp[]): PendingOp[] {
  return ops.flatMap((op): PendingOp[] => {
    if ((op.kind !== 'add-option' && op.kind !== 'add-value') || !op.channelPrices || Object.keys(op.channelPrices).length === 0) {
      return [op]
    }
    const { channelPrices, ...add } = op
    return op.kind === 'add-option'
      ? [add as AddOptionOp, { kind: 'update-option', id: op.tempId, channelPrices }]
      : [add as AddValueOp, { kind: 'update-value', id: op.tempId, channelPrices }]
  })
}

const onlyChannelPrices = (op: UpdateOptionOp | UpdateValueOp) =>
  Object.keys(op).every((k) => k === 'kind' || k === 'id' || k === 'channelPrices')

// Thrown when a mid-sequence mutation fails - carries the unapplied tail (temp refs already resolved) so the section
// can keep just that and a retry resumes instead of re-running committed adds and duplicating rows
export class CommitError extends Error {
  remaining: PendingOp[]
  constructor(cause: unknown, remaining: PendingOp[]) {
    super(cause instanceof Error ? cause.message : String(cause))
    this.name = 'CommitError'
    this.cause = cause
    this.remaining = remaining
  }
}

export async function commitOps(
  staged: readonly PendingOp[],
  productId: string,
  committers: Committers,
): Promise<{ idMap: Map<string, string> }> {
  const ops = splitChannelPrices(staged)
  const idMap = new Map<string, string>()
  const resolve = (id: string) => idMap.get(id) ?? id

  for (let i = 0; i < ops.length; i++) {
    try {
      await commitOne(ops[i], productId, committers, idMap, resolve)
    } catch (err) {
      const remaining = ops.slice(i).map((o): PendingOp => {
        if (o.kind === 'add-option') return o
        if (o.kind === 'add-value') return { ...o, optionRef: resolve(o.optionRef) }
        return { ...o, id: resolve(o.id) }
      })
      throw new CommitError(err, remaining)
    }
  }

  return { idMap }
}

async function commitOne(
  op: PendingOp,
  productId: string,
  committers: Committers,
  idMap: Map<string, string>,
  resolve: (id: string) => string,
): Promise<void> {
  switch (op.kind) {
    case 'add-option': {
      const realId = await committers.addOption({ ...op, productId })
      idMap.set(op.tempId, realId)
      break
    }
    case 'update-option': {
      const id = resolve(op.id)
      if (!onlyChannelPrices(op)) await committers.updateOption({ ...op, id })
      // Title + help text are per-locale content - The form sends both whenever either changes (a content upsert
      // replaces the row), so editing help text alone cannot drop the title
      if (op.titleLocale && (op.title !== undefined || op.helpText !== undefined)) {
        await committers.upsertOptionContent({
          optionId: id,
          locale: op.titleLocale,
          title: op.title ?? '',
          helpText: op.helpText ?? '',
        })
      }
      for (const [channelId, priceValue] of Object.entries(op.channelPrices ?? {})) {
        await committers.setOptionChannelPrice({ optionId: id, channelId, priceValue })
      }
      break
    }
    case 'delete-option': {
      const id = resolve(op.id)
      if (isTemp(id)) break
      await committers.deleteOption({ id })
      break
    }
    case 'add-value': {
      const optionId = resolve(op.optionRef)
      const realId = await committers.addValue({ ...op, optionRef: optionId })
      idMap.set(op.tempId, realId)
      break
    }
    case 'update-value': {
      const id = resolve(op.id)
      if (!onlyChannelPrices(op)) await committers.updateValue({ ...op, id })
      if (op.title !== undefined && op.titleLocale) {
        await committers.upsertValueContent({ valueId: id, locale: op.titleLocale, title: op.title })
      }
      for (const [channelId, priceValue] of Object.entries(op.channelPrices ?? {})) {
        await committers.setValueChannelPrice({ valueId: id, channelId, priceValue })
      }
      break
    }
    case 'delete-value': {
      const id = resolve(op.id)
      if (isTemp(id)) break
      await committers.deleteValue({ id })
      break
    }
  }
}
