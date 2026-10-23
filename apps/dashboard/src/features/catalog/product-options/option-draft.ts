import type { ProductOptionInputType, ProductOptionPriceType } from '@/graphql/schema.types'
import { formatCents, fromMinor, toMinor } from '@/lib/format'
import { channelEditsFrom, invalidChannelPrices, rawChannelPrices, type SurchargeChannel } from '../channelSurcharges'
import {
  conditionReferences,
  newTempId,
  optionContentFor,
  parseVisibleIfGroup,
  valueContentFor,
  type AddValueOp,
  type OptionImageRef,
  type OptionValue,
  type PendingOp,
  type ProductOption,
  type SourceProduct,
  type UpdateOptionOp,
  type UpdateValueOp,
  type VisibilityGroup,
  type VisibilityRule,
} from '../productOptionsStaging'
import { INPUT_TYPES, isChoice } from './option-types'

// The option dialog edits a draft of plain strings, the way they're typed, and turns it into staged ops only on Done -
// comparing against the draft it opened with, so "3" retyped as "3.00" is not a change

export type DraftContext = { baseCurrency: string; channels: readonly SurchargeChannel[] }

export type ChoiceDraft = {
  key: string
  // The value it edits, or null for one added in this dialog
  id: string | null
  title: string
  price: string
  priceType: ProductOptionPriceType
  isDefault: boolean
  quantity: string
  lockQty: string
  sourceProduct: SourceProduct | null
  channelPrices: Record<string, string>
  image: OptionImageRef | null
}

export type OptionDraft = {
  title: string
  helpText: string
  inputType: ProductOptionInputType
  isRequired: boolean
  price: string
  priceType: ProductOptionPriceType
  channelPrices: Record<string, string>
  maxChars: string
  maxFileMb: string
  fileExtensions: string
  logic: 'and' | 'or'
  rules: VisibilityRule[]
  choices: ChoiceDraft[]
  image: OptionImageRef | null
}

const MB = 1024 * 1024

export const priceText = (minor: number, type: ProductOptionPriceType, currency: string) =>
  type === 'PERCENT' ? String(minor) : fromMinor(minor, currency)

// null for blank or unreadable - fixed amounts in the currency's own decimals, percentages whole
export function parsePrice(text: string, type: ProductOptionPriceType, currency: string): number | null {
  const t = text.trim()
  if (t === '') return null
  if (type === 'FIXED') return toMinor(t, currency)
  const n = Number(t)
  return Number.isInteger(n) ? n : null
}

export function newChoice(): ChoiceDraft {
  return { key: newTempId(), id: null, title: '', price: '', priceType: 'FIXED', isDefault: false, quantity: '1', lockQty: '', sourceProduct: null, channelPrices: {}, image: null }
}

export function choiceFromProduct(p: SourceProduct): ChoiceDraft {
  return { ...newChoice(), title: p.name || p.sku || '', sourceProduct: p }
}

export const isBlankChoice = (c: ChoiceDraft) => c.id === null && !c.title.trim() && !c.price.trim() && !c.sourceProduct

function choiceFromValue(v: OptionValue, locale: string, ctx: DraftContext): ChoiceDraft {
  return {
    key: v.id,
    id: v.id,
    title: valueContentFor(v, locale),
    price: priceText(v.priceValue, v.sourceProduct ? 'FIXED' : v.priceType, ctx.baseCurrency),
    priceType: v.sourceProduct ? 'FIXED' : v.priceType,
    isDefault: v.isDefault,
    quantity: String(v.quantity),
    lockQty: v.forcedLineQty != null ? String(v.forcedLineQty) : '',
    sourceProduct: v.sourceProduct ?? null,
    channelPrices: rawChannelPrices(v.channelPrices, ctx.channels),
    image: v.image ?? null,
  }
}

export function draftFromOption(option: ProductOption | null, locale: string, ctx: DraftContext): OptionDraft {
  if (!option) {
    return {
      title: '', helpText: '', inputType: 'DROPDOWN', isRequired: false, price: '', priceType: 'FIXED', channelPrices: {},
      maxChars: '', maxFileMb: '', fileExtensions: '', logic: 'and', rules: [], choices: [newChoice(), newChoice()], image: null,
    }
  }
  const content = optionContentFor(option, locale)
  const vis = parseVisibleIfGroup(option.visibleIf)
  return {
    title: content.title,
    helpText: content.helpText,
    inputType: option.inputType,
    isRequired: option.isRequired,
    price: option.priceValue == null ? '' : priceText(option.priceValue, option.priceType ?? 'FIXED', ctx.baseCurrency),
    priceType: option.priceType ?? 'FIXED',
    channelPrices: rawChannelPrices(option.channelPrices, ctx.channels),
    maxChars: option.maxChars == null ? '' : String(option.maxChars),
    maxFileMb: option.maxFileBytes == null ? '' : String(Number((option.maxFileBytes / MB).toFixed(2))),
    fileExtensions: option.fileExtensions ?? '',
    logic: vis?.logic === 'or' ? 'or' : 'and',
    rules: vis?.rules ?? [],
    choices: [...option.values].sort((a, b) => a.sortOrder - b.sortOrder).map((v) => choiceFromValue(v, locale, ctx)),
    image: option.image ?? null,
  }
}

// ─── Validation ─────────────────────────────────────────────────────

export type ChoiceErrors = { title?: string; price?: string; quantity?: string; lockQty?: string; channels?: string }
export type DraftErrors = {
  title?: string
  price?: string
  channels?: string
  maxChars?: string
  maxFileMb?: string
  choices?: string
  rules?: string
  choice: Record<string, ChoiceErrors>
}

const priceError = (type: ProductOptionPriceType, currency: string) =>
  type === 'PERCENT' ? 'Use a whole percentage, like 10.' : `Enter an amount in ${currency}, like 2.50.`

const whole = (t: string) => Number.isInteger(Number(t)) && Number(t) >= 1

export function validateDraft(d: OptionDraft, ctx: DraftContext): DraftErrors | null {
  const e: DraftErrors = { choice: {} }
  const cur = ctx.baseCurrency
  if (!d.title.trim()) e.title = 'Give the option a name customers will see.'
  if (isChoice(d.inputType)) {
    const live = d.choices.filter((c) => !isBlankChoice(c))
    if (live.length === 0) e.choices = 'Add at least one choice.'
    for (const c of live) {
      const ce: ChoiceErrors = {}
      const type = c.sourceProduct ? 'FIXED' : c.priceType
      if (!c.title.trim()) ce.title = 'Name this choice.'
      if (c.price.trim() !== '' && parsePrice(c.price, type, cur) === null) ce.price = priceError(type, cur)
      if (!(Number(c.quantity) >= 1)) ce.quantity = 'At least 1.'
      if (c.lockQty.trim() !== '' && !whole(c.lockQty)) ce.lockQty = 'A whole number, 1 or more.'
      if (invalidChannelPrices(ctx.channels, c.channelPrices).length > 0) ce.channels = 'Fix the store amounts.'
      if (Object.keys(ce).length > 0) e.choice[c.key] = ce
    }
  } else {
    if (d.price.trim() !== '' && parsePrice(d.price, d.priceType, cur) === null) e.price = priceError(d.priceType, cur)
    if (d.priceType === 'FIXED' && invalidChannelPrices(ctx.channels, d.channelPrices).length > 0) e.channels = 'Fix the store amounts.'
    if (INPUT_TYPES[d.inputType].family === 'text' && d.maxChars.trim() !== '' && !whole(d.maxChars)) e.maxChars = 'A whole number, 1 or more.'
    if (d.inputType === 'FILE' && d.maxFileMb.trim() !== '' && !(Number(d.maxFileMb) > 0)) e.maxFileMb = 'A size above 0.'
  }
  if (d.rules.some((r) => !r.option_id || ((r.op === 'anyOf' || r.op === 'noneOf') && (r.value_ids?.length ?? 0) === 0))) {
    e.rules = 'Finish or remove the incomplete condition.'
  }
  const { choice, ...rest } = e
  return Object.keys(rest).length > 0 || Object.keys(choice).length > 0 ? e : null
}

// ─── Draft to ops ───────────────────────────────────────────────────

export function conditionFrom(d: Pick<OptionDraft, 'logic' | 'rules'>): VisibilityGroup | null {
  if (d.rules.length === 0) return null
  const rules = d.rules.map((r): VisibilityRule =>
    r.op === 'anyOf' || r.op === 'noneOf' ? { option_id: r.option_id, op: r.op, value_ids: r.value_ids ?? [] } : { option_id: r.option_id, op: r.op },
  )
  return d.logic === 'or' ? { logic: 'or', rules } : { rules }
}

const extensions = (t: string) => t.split(/[\s,]+/).map((x) => x.replace(/^\./, '').toLowerCase()).filter(Boolean).join(',')

function addValueOp(c: ChoiceDraft, optionRef: string, sortOrder: number, locale: string, ctx: DraftContext): AddValueOp {
  const priceType = c.sourceProduct ? 'FIXED' : c.priceType
  const edits = priceType === 'FIXED' ? channelEditsFrom(ctx.channels, c.channelPrices, []) : undefined
  return {
    kind: 'add-value',
    tempId: newTempId(),
    optionRef,
    sortOrder,
    priceValue: parsePrice(c.price, priceType, ctx.baseCurrency) ?? 0,
    priceType,
    isDefault: c.isDefault,
    quantity: Number(c.quantity) || 1,
    forcedLineQty: c.lockQty.trim() === '' ? null : Number(c.lockQty),
    title: c.title.trim(),
    locale,
    sourceProductId: c.sourceProduct?.id ?? null,
    sourceProduct: c.sourceProduct,
    ...(c.image ? { image: c.image } : {}),
    ...(edits ? { channelPrices: edits } : {}),
  }
}

export function draftToOps({
  draft: d,
  initial: i,
  option,
  locale,
  contentLocale,
  sortOrder,
  ctx,
}: {
  draft: OptionDraft
  initial: OptionDraft
  // The option being edited - null for a new one
  option: ProductOption | null
  // The language the copy fields were edited in
  locale: string
  // The language new options and choices are created in - the backend requires the default
  contentLocale: string
  sortOrder: number
  ctx: DraftContext
}): PendingOp[] {
  const cur = ctx.baseCurrency
  const choice = isChoice(d.inputType)
  const family = INPUT_TYPES[d.inputType].family
  const live = choice ? d.choices.filter((c) => !isBlankChoice(c)) : []
  const price = choice ? null : parsePrice(d.price, d.priceType, cur)

  if (!option) {
    const tempId = newTempId()
    const edits = price !== null && d.priceType === 'FIXED' ? channelEditsFrom(ctx.channels, d.channelPrices, []) : undefined
    return [
      {
        kind: 'add-option',
        tempId,
        inputType: d.inputType,
        isRequired: d.isRequired,
        sortOrder,
        title: d.title.trim(),
        helpText: d.helpText.trim(),
        locale: contentLocale,
        priceValue: price,
        priceType: price === null ? null : d.priceType,
        maxChars: family === 'text' && d.maxChars.trim() !== '' ? Number(d.maxChars) : null,
        maxFileBytes: d.inputType === 'FILE' && d.maxFileMb.trim() !== '' ? Math.round(Number(d.maxFileMb) * MB) : null,
        fileExtensions: d.inputType === 'FILE' && extensions(d.fileExtensions) ? extensions(d.fileExtensions) : null,
        visibleIf: conditionFrom(d),
        ...(d.image ? { image: d.image } : {}),
        ...(edits ? { channelPrices: edits } : {}),
      },
      ...live.map((c, n) => addValueOp(c, tempId, n, contentLocale, ctx)),
    ]
  }

  const ops: PendingOp[] = []
  const upd: UpdateOptionOp = { kind: 'update-option', id: option.id }
  if (d.title.trim() !== i.title.trim() || d.helpText.trim() !== i.helpText.trim()) {
    Object.assign(upd, { title: d.title.trim(), helpText: d.helpText.trim(), titleLocale: locale })
  }
  if (d.inputType !== i.inputType) upd.inputType = d.inputType
  if (d.isRequired !== i.isRequired) upd.isRequired = d.isRequired
  if (!choice) {
    if (d.price.trim() !== i.price.trim() || d.priceType !== i.priceType) {
      upd.priceValue = price
      if (price !== null) upd.priceType = d.priceType
    }
    const edits = price !== null && d.priceType === 'FIXED' ? channelEditsFrom(ctx.channels, d.channelPrices, option.channelPrices) : undefined
    if (edits) upd.channelPrices = edits
    if (family === 'text' && d.maxChars.trim() !== i.maxChars.trim()) upd.maxChars = d.maxChars.trim() === '' ? null : Number(d.maxChars)
    if (d.inputType === 'FILE' && d.maxFileMb.trim() !== i.maxFileMb.trim()) {
      upd.maxFileBytes = d.maxFileMb.trim() === '' ? null : Math.round(Number(d.maxFileMb) * MB)
    }
    if (d.inputType === 'FILE' && extensions(d.fileExtensions) !== extensions(i.fileExtensions)) {
      upd.fileExtensions = extensions(d.fileExtensions) || null
    }
  }
  const next = conditionFrom(d)
  if (JSON.stringify(next) !== JSON.stringify(conditionFrom(i))) upd.visibleIf = next
  if ((d.image?.assetId ?? null) !== (i.image?.assetId ?? null)) upd.image = d.image
  if (Object.keys(upd).length > 2) ops.push(upd)

  if (!choice) return ops
  const keptIds = new Set(live.map((c) => c.id).filter(Boolean))
  for (const c of i.choices) if (c.id && !keptIds.has(c.id)) ops.push({ kind: 'delete-value', id: c.id })
  const before = i.choices.map((c) => c.id).join(',')
  const reordered = live.map((c) => c.id ?? 'new').join(',') !== before
  const values = new Map(option.values.map((v) => [v.id, v]))
  const initialChoices = new Map(i.choices.map((c) => [c.key, c]))
  live.forEach((c, n) => {
    if (!c.id) {
      ops.push(addValueOp(c, option.id, n, contentLocale, ctx))
      return
    }
    const v = values.get(c.id)
    const ic = initialChoices.get(c.key)
    if (!v || !ic) return
    const vu: UpdateValueOp = { kind: 'update-value', id: c.id }
    const type = c.sourceProduct ? 'FIXED' : c.priceType
    if (c.title.trim() !== ic.title.trim()) Object.assign(vu, { title: c.title.trim(), titleLocale: locale })
    if (c.price.trim() !== ic.price.trim() || c.priceType !== ic.priceType || c.sourceProduct?.id !== ic.sourceProduct?.id) {
      const pv = parsePrice(c.price, type, cur) ?? 0
      if (pv !== v.priceValue) vu.priceValue = pv
      if (type !== v.priceType) vu.priceType = type
    }
    if (c.isDefault !== v.isDefault) vu.isDefault = c.isDefault
    if (c.quantity.trim() !== ic.quantity.trim()) vu.quantity = Number(c.quantity) || 1
    if (c.lockQty.trim() !== ic.lockQty.trim()) vu.forcedLineQty = c.lockQty.trim() === '' ? null : Number(c.lockQty)
    if ((c.sourceProduct?.id ?? null) !== (v.sourceProduct?.id ?? null)) {
      vu.sourceProductId = c.sourceProduct?.id ?? null
      vu.sourceProduct = c.sourceProduct
    }
    if ((c.image?.assetId ?? null) !== (v.image?.assetId ?? null)) vu.image = c.image
    const edits = type === 'FIXED' ? channelEditsFrom(ctx.channels, c.channelPrices, v.channelPrices) : undefined
    if (edits) vu.channelPrices = edits
    if (reordered && v.sortOrder !== n) vu.sortOrder = n
    if (Object.keys(vu).length > 2) ops.push(vu)
  })
  return ops
}

// ─── Reading an option back ─────────────────────────────────────────

// "+$3.00", "+10%", "−$1.00" - null when the choice costs nothing extra
export function priceLabel(value: number | null | undefined, type: ProductOptionPriceType | null | undefined, currency: string): string | null {
  if (value == null || value === 0) return null
  const sign = value > 0 ? '+' : '−'
  return type === 'PERCENT' ? `${sign}${Math.abs(value)}%` : `${sign}${formatCents(Math.abs(value), currency)}`
}

// "Wrapping is Gift or Premium, and Note is filled in" - null for no condition
export function conditionText(raw: unknown, all: readonly ProductOption[], locale: string): string | null {
  const g = parseVisibleIfGroup(raw)
  if (!g) return null
  if ((g.groups?.length ?? 0) > 0) return 'a custom condition'
  const parts = (g.rules ?? []).map((r) => {
    const parent = all.find((o) => o.id === r.option_id)
    const name = parent ? optionContentFor(parent, locale).title || 'Untitled option' : 'a removed option'
    const values = (r.value_ids ?? []).map((id) => {
      const v = parent?.values.find((x) => x.id === id)
      return v ? valueContentFor(v, locale) : 'a removed choice'
    })
    switch (r.op) {
      case 'filled':
        return `${name} is filled in`
      case 'empty':
        return `${name} is left empty`
      case 'anyOf':
        return `${name} is ${values.join(' or ')}`
      case 'noneOf':
        return `${name} is not ${values.join(' or ')}`
    }
  })
  return parts.length > 0 ? parts.join(g.logic === 'or' ? ', or ' : ', and ') : null
}

export function conditionSummary(option: ProductOption, all: readonly ProductOption[], locale: string): string | null {
  const text = conditionText(option.visibleIf, all, locale)
  return text === null ? null : text === 'a custom condition' ? 'Shown under a custom condition' : `Only when ${text}`
}

// Which options' conditions point at this one, or at one of its choices - what blocks deleting either
export function dependents(all: readonly ProductOption[], ref: { optionId?: string; valueId?: string }): ProductOption[] {
  return all.filter((o) => o.id !== ref.optionId && conditionReferences(o.visibleIf, ref))
}
