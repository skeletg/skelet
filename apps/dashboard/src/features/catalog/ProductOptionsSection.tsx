import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { CopyIcon, EyeIcon, LinkIcon, PencilIcon, PlusIcon, SlidersIcon, Trash2Icon, XIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { ProductOptionInputType, ProductOptionPriceType } from '@/graphql/schema.types'
import {
  AddProductOptionDocument,
  AddProductOptionValueDocument,
  DeleteProductOptionContentDocument,
  DeleteProductOptionDocument,
  DeleteProductOptionValueContentDocument,
  DeleteProductOptionValueDocument,
  DuplicateProductOptionDocument,
  ProductDocument,
  ProductPickerDocument,
  SetProductOptionChannelPriceDocument,
  SetProductOptionValueChannelPriceDocument,
  UpdateProductOptionDocument,
  UpdateProductOptionValueDocument,
  UpsertProductOptionContentDocument,
  UpsertProductOptionValueContentDocument,
} from './Catalog.gql'
import { TranslationLocales } from '@/components/TranslationLocales'
import {
  applyOps,
  CommitError,
  commitOps,
  isTemp,
  newTempId,
  opsToDiff,
  optionContentFor,
  reduceOps,
  valueContentFor,
  type Committers,
  type OptionValue,
  parseVisibleIfGroup,
  type PendingOp,
  type ProductOption,
  type SourceProduct,
  type VisibilityGroup,
  type VisibilityRule,
} from './productOptionsStaging'
import { useProductSaveSection } from './productSaveRegistry'
import { useConfigScope } from '@/lib/config-scope'
import { ChannelSurchargeInputs, channelEditsFrom, rawChannelPrices, type SurchargeChannel } from './ChannelSurchargeInputs'

type WithOptions = {
  id: string
  contentLocale?: string | null
  defaultCurrencyCode?: string
  listings?: { channel: SurchargeChannel }[]
  productOptions: ProductOption[]
}

type Surcharges = { channels: SurchargeChannel[]; baseCurrency: string }

const INPUT_TYPE_LABEL: Record<ProductOptionInputType, string> = {
  FIELD: 'Text · field',
  AREA: 'Text · area',
  DROPDOWN: 'Dropdown',
  RADIO: 'Radio',
  CHECKBOX: 'Checkbox',
  MULTISELECT: 'Multiselect',
  DATE: 'Date',
  DATE_TIME: 'Date & time',
  TIME: 'Time',
  FILE: 'File upload',
}

const INPUT_TYPES: ProductOptionInputType[] = [
  'FIELD',
  'AREA',
  'DROPDOWN',
  'RADIO',
  'CHECKBOX',
  'MULTISELECT',
  'DATE',
  'DATE_TIME',
  'TIME',
  'FILE',
]

const SELECT_TYPES: ProductOptionInputType[] = ['DROPDOWN', 'RADIO', 'CHECKBOX', 'MULTISELECT']
const SINGLE_SELECT_TYPES: ProductOptionInputType[] = ['DROPDOWN', 'RADIO']
const TEXT_TYPES: ProductOptionInputType[] = ['FIELD', 'AREA']
const FILE_TYPES: ProductOptionInputType[] = ['FILE']

function isSelect(t: ProductOptionInputType) {
  return SELECT_TYPES.includes(t)
}
function isSingleSelect(t: ProductOptionInputType) {
  return SINGLE_SELECT_TYPES.includes(t)
}
function isText(t: ProductOptionInputType) {
  return TEXT_TYPES.includes(t)
}
function isFile(t: ProductOptionInputType) {
  return FILE_TYPES.includes(t)
}

export function ProductOptionsSection({
  product,
  canManage,
}: {
  product: WithOptions
  canManage: boolean
}) {
  const [pendingOps, setPendingOps] = useState<PendingOp[]>([])

  const stage = useCallback((op: PendingOp) => {
    setPendingOps((prev) => reduceOps([...prev, op]))
  }, [])

  const reduced = useMemo(() => reduceOps(pendingOps), [pendingOps])
  const staged = useMemo(() => applyOps(product.productOptions, reduced), [product.productOptions, reduced])
  const surcharges = useMemo<Surcharges>(
    () => ({
      channels: (product.listings ?? []).map((l) => l.channel),
      baseCurrency: product.defaultCurrencyCode ?? '',
    }),
    [product.listings, product.defaultCurrencyCode],
  )
  const diff = useMemo(() => {
    const byId = new Map(surcharges.channels.map((c) => [c.id, c]))
    const label = (id: string) => {
      const c = byId.get(id)
      return c ? `${c.name} (${c.currencyCode})` : 'another channel'
    }
    return opsToDiff(product.productOptions, reduced, label)
  }, [product.productOptions, reduced, surcharges.channels])

  // New options/values must be created in the configured default locale (the backend enforces this); translations are
  // added afterwards by switching the edit locale
  const defaultLocale = product.contentLocale ?? 'en'
  const { effectiveLocale, availableLocales } = useConfigScope()
  // The locale whose copy the table + edit dialogs show - Defaults to the dashboard's active locale; the picker lets an
  // editor target any configured (or already-translated) locale
  const [editLocale, setEditLocale] = useState(effectiveLocale || defaultLocale)
  const localeOptions = useMemo(() => {
    const seen = new Set<string>()
    const out: string[] = []
    const push = (l: string) => {
      if (l && !seen.has(l)) { seen.add(l); out.push(l) }
    }
    push(defaultLocale)
    for (const l of availableLocales) push(l)
    for (const o of product.productOptions) for (const c of o.content ?? []) push(c.locale)
    push(editLocale)
    return out
  }, [availableLocales, product.productOptions, defaultLocale, editLocale])

  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [addOptionMutation] = useMutation(AddProductOptionDocument, { refetchQueries: refetch })
  const [updateOptionMutation] = useMutation(UpdateProductOptionDocument, { refetchQueries: refetch })
  const [upsertOptionContentMutation] = useMutation(UpsertProductOptionContentDocument, { refetchQueries: refetch })
  const [deleteOptionMutation] = useMutation(DeleteProductOptionDocument, { refetchQueries: refetch })
  const [addValueMutation] = useMutation(AddProductOptionValueDocument, { refetchQueries: refetch })
  const [updateValueMutation] = useMutation(UpdateProductOptionValueDocument, { refetchQueries: refetch })
  const [upsertValueContentMutation] = useMutation(UpsertProductOptionValueContentDocument, { refetchQueries: refetch })
  const [deleteValueMutation] = useMutation(DeleteProductOptionValueDocument, { refetchQueries: refetch })
  const [duplicateOptionMutation] = useMutation(DuplicateProductOptionDocument, { refetchQueries: refetch })
  const [setOptionChannelPriceMutation] = useMutation(SetProductOptionChannelPriceDocument, { refetchQueries: refetch })
  const [setValueChannelPriceMutation] = useMutation(SetProductOptionValueChannelPriceDocument, { refetchQueries: refetch })

  // Duplicate fires immediately (not staged) - it's a server-side
  // clone that doesn't interact with the section's pending ops
  const onDuplicateOption = useCallback(
    async (id: string) => {
      if (isTemp(id)) return // staged option not committed yet - re-stage manually
      await duplicateOptionMutation({ variables: { id } })
    },
    [duplicateOptionMutation],
  )

  const save = useCallback(async () => {
    if (reduced.length === 0) return
    const knownOptionIds = new Set(product.productOptions.map((o) => o.id))
    const knownValueIds = new Set<string>()
    for (const o of product.productOptions) for (const v of o.values) knownValueIds.add(v.id)

    const committers: Committers = {
      addOption: async (op) => {
        const res = await addOptionMutation({
          variables: {
            input: {
              productId: op.productId,
              inputType: op.inputType,
              isRequired: op.isRequired,
              sortOrder: op.sortOrder,
              priceValue: op.priceValue ?? undefined,
              priceType: op.priceType ?? undefined,
              maxChars: op.maxChars ?? undefined,
              maxFileBytes: op.maxFileBytes ?? undefined,
              fileExtensions: op.fileExtensions ?? undefined,
              visibleIf: op.visibleIf ?? undefined,
              locale: op.locale,
              title: op.title,
              helpText: op.helpText || undefined,
            },
          },
        })
        const opts = res.data?.addProductOption.productOptions ?? []
        const fresh = opts.find((o) => !knownOptionIds.has(o.id))
        if (!fresh) throw new Error('Could not resolve newly-added option id from response.')
        knownOptionIds.add(fresh.id)
        return fresh.id
      },
      updateOption: async (op) => {
        const variables: {
          input: {
            id: string
            inputType?: ProductOptionInputType
            isRequired?: boolean
            sortOrder?: number
            priceValue?: number
            priceType?: ProductOptionPriceType
            maxChars?: number
            maxFileBytes?: number
            fileExtensions?: string
            clearPrice?: boolean
            clearMaxChars?: boolean
            clearMaxFileBytes?: boolean
            clearFileExtensions?: boolean
            visibleIf?: VisibilityGroup
            clearVisibleIf?: boolean
          }
        } = { input: { id: op.id } }
        if (op.inputType !== undefined) variables.input.inputType = op.inputType
        if (op.isRequired !== undefined) variables.input.isRequired = op.isRequired
        if (op.sortOrder !== undefined) variables.input.sortOrder = op.sortOrder
        // null in a staged op means "clear" - the wire's null is a silent
        // no-op, so it has to become the matching clear flag instead
        if (op.priceValue !== undefined) {
          if (op.priceValue === null) variables.input.clearPrice = true
          else variables.input.priceValue = op.priceValue
        }
        if (op.priceType !== undefined && op.priceType !== null && op.priceValue !== null) {
          variables.input.priceType = op.priceType
        }
        if (op.maxChars !== undefined) {
          if (op.maxChars === null) variables.input.clearMaxChars = true
          else variables.input.maxChars = op.maxChars
        }
        if (op.maxFileBytes !== undefined) {
          if (op.maxFileBytes === null) variables.input.clearMaxFileBytes = true
          else variables.input.maxFileBytes = op.maxFileBytes
        }
        if (op.fileExtensions !== undefined) {
          if (op.fileExtensions === null) variables.input.clearFileExtensions = true
          else variables.input.fileExtensions = op.fileExtensions
        }
        if (op.visibleIf !== undefined) {
          if (op.visibleIf === null) variables.input.clearVisibleIf = true
          else variables.input.visibleIf = op.visibleIf
        }
        await updateOptionMutation({ variables })
      },
      upsertOptionContent: async (op) => {
        await upsertOptionContentMutation({
          variables: { optionId: op.optionId, locale: op.locale, title: op.title, helpText: op.helpText },
        })
      },
      deleteOption: async (op) => {
        await deleteOptionMutation({ variables: { id: op.id } })
      },
      addValue: async (op) => {
        const res = await addValueMutation({
          variables: {
            input: {
              optionId: op.optionRef,
              sortOrder: op.sortOrder,
              priceValue: op.priceValue,
              priceType: op.priceType,
              isDefault: op.isDefault,
              quantity: op.quantity,
              forcedLineQty: op.forcedLineQty ?? undefined,
              locale: op.locale,
              title: op.title,
              sourceProductId: op.sourceProductId ?? undefined,
            },
          },
        })
        const opts = res.data?.addProductOptionValue.productOptions ?? []
        let fresh: { id: string } | undefined
        for (const o of opts) for (const v of o.values) {
          if (!knownValueIds.has(v.id)) { fresh = v; break }
        }
        if (!fresh) throw new Error('Could not resolve newly-added value id from response.')
        knownValueIds.add(fresh.id)
        return fresh.id
      },
      updateValue: async (op) => {
        const variables: {
          input: {
            id: string
            sortOrder?: number
            priceValue?: number
            priceType?: ProductOptionPriceType
            isDefault?: boolean
            quantity?: number
            forcedLineQty?: number
            sourceProductId?: string
          }
        } = { input: { id: op.id } }
        if (op.sortOrder !== undefined) variables.input.sortOrder = op.sortOrder
        if (op.priceValue !== undefined) variables.input.priceValue = op.priceValue
        if (op.priceType !== undefined) variables.input.priceType = op.priceType
        if (op.isDefault !== undefined) variables.input.isDefault = op.isDefault
        if (op.quantity !== undefined) variables.input.quantity = op.quantity
        if (op.forcedLineQty !== undefined) {
          // null = clear; the schema's sentinel for clearing is <= 0
          variables.input.forcedLineQty = op.forcedLineQty ?? 0
        }
        if (op.sourceProductId !== undefined) {
          // null = unlink (sent as empty string per the schema's
          // documented sentinel); a real uuid is sent verbatim
          variables.input.sourceProductId = op.sourceProductId ?? ''
        }
        await updateValueMutation({ variables })
      },
      upsertValueContent: async (op) => {
        await upsertValueContentMutation({
          variables: { valueId: op.valueId, locale: op.locale, title: op.title },
        })
      },
      deleteValue: async (op) => {
        await deleteValueMutation({ variables: { id: op.id } })
      },
      setOptionChannelPrice: async (op) => {
        await setOptionChannelPriceMutation({ variables: op })
      },
      setValueChannelPrice: async (op) => {
        await setValueChannelPriceMutation({ variables: op })
      },
    }

    try {
      await commitOps(reduced, product.id, committers)
    } catch (e) {
      // keep only the unapplied tail - a retry resumes instead of
      // re-running committed adds and duplicating options/values
      if (e instanceof CommitError) setPendingOps(e.remaining)
      throw e
    }
    setPendingOps([])
  }, [
    reduced,
    product.id,
    product.productOptions,
    addOptionMutation,
    updateOptionMutation,
    upsertOptionContentMutation,
    deleteOptionMutation,
    addValueMutation,
    updateValueMutation,
    upsertValueContentMutation,
    deleteValueMutation,
    setOptionChannelPriceMutation,
    setValueChannelPriceMutation,
  ])

  useProductSaveSection({ id: 'product-options', title: 'Product options', diff, save })

  // Inline (non-modal) editors: at most one open at a time. valueForm
  // with a null valueId is the "new value" form for that option
  const [addingOption, setAddingOption] = useState(false)
  const [editingOptionId, setEditingOptionId] = useState<string | null>(null)
  const [valueForm, setValueForm] = useState<{ optionId: string; valueId: string | null } | null>(null)

  const sortedOptions = useMemo(() => [...staged].sort((a, b) => a.sortOrder - b.sortOrder), [staged])

  return (
    <div className="grid gap-4">
      {/* Title + description come from the parent CollapsibleSection -
          this row is just the section toolbar */}
      <div className="flex items-center justify-end">
        <div className="flex items-center gap-2">
          {localeOptions.length > 1 && (
            <Select value={editLocale} onValueChange={setEditLocale}>
              <SelectTrigger size="sm" className="w-[8rem]" aria-label="Edit locale">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {localeOptions.map((l) => (
                  <SelectItem key={l} value={l}>
                    {l}
                    {l === defaultLocale ? ' (default)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          {canManage && (
            <Button
              size="sm"
              onClick={() => {
                setAddingOption(true)
                setEditingOptionId(null)
                setValueForm(null)
              }}
            >
              <PlusIcon className="size-4" />
              Add option
            </Button>
          )}
        </div>
      </div>

      {sortedOptions.length === 0 && !addingOption ? (
        <p className="rounded-md border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
          No options yet. Add one to start collecting customer input at add-to-cart.
        </p>
      ) : (
        <div className="divide-y overflow-hidden rounded-md border">
          {sortedOptions.map((opt) => {
            if (canManage && editingOptionId === opt.id) {
              return (
                <EditOptionForm
                  key={`${opt.id}:${editLocale}`}
                  option={opt}
                  siblings={sortedOptions}
                  locale={editLocale}
                  defaultLocale={defaultLocale}
                  productId={product.id}
                  surcharges={surcharges}
                  onStage={stage}
                  onClose={() => setEditingOptionId(null)}
                />
              )
            }
            const editingVal =
              canManage && valueForm?.optionId === opt.id && valueForm.valueId !== null
                ? opt.values.find((x) => x.id === valueForm.valueId)
                : undefined
            return (
              <OptionCard
                key={opt.id}
                option={opt}
                locale={editLocale}
                canManage={canManage}
                onEdit={() => {
                  setEditingOptionId(opt.id)
                  setAddingOption(false)
                  setValueForm(null)
                }}
                onDelete={() => stage({ kind: 'delete-option', id: opt.id })}
                onDuplicate={() => onDuplicateOption(opt.id)}
                onAddValue={() => setValueForm({ optionId: opt.id, valueId: null })}
                onEditValue={(v) => setValueForm({ optionId: opt.id, valueId: v.id })}
                onDeleteValue={(v) => stage({ kind: 'delete-value', id: v.id })}
                valueFormSlot={
                  canManage && valueForm?.optionId === opt.id ? (
                    valueForm.valueId === null ? (
                      <AddValueForm
                        option={opt}
                        locale={defaultLocale}
                        surcharges={surcharges}
                        onStage={stage}
                        onClose={() => setValueForm(null)}
                      />
                    ) : editingVal ? (
                      <EditValueForm
                        key={`${editingVal.id}:${editLocale}`}
                        value={editingVal}
                        locale={editLocale}
                        defaultLocale={defaultLocale}
                        productId={product.id}
                        surcharges={surcharges}
                        onStage={stage}
                        onClose={() => setValueForm(null)}
                      />
                    ) : null
                  ) : null
                }
              />
            )
          })}
        </div>
      )}

      {canManage && addingOption && (
        <AddOptionForm
          locale={defaultLocale}
          existingMaxSort={sortedOptions.reduce((m, o) => Math.max(m, o.sortOrder), -1)}
          surcharges={surcharges}
          onStage={stage}
          onClose={() => setAddingOption(false)}
        />
      )}

    </div>
  )
}

function OptionCard({
  option,
  locale,
  canManage,
  onEdit,
  onDelete,
  onDuplicate,
  onAddValue,
  onEditValue,
  onDeleteValue,
  valueFormSlot,
}: {
  option: ProductOption
  locale: string
  canManage: boolean
  onEdit: () => void
  onDelete: () => void
  onDuplicate: () => void
  onAddValue: () => void
  onEditValue: (v: OptionValue) => void
  onDeleteValue: (v: OptionValue) => void
  valueFormSlot?: ReactNode
}) {
  const pending = isTemp(option.id)
  const content = optionContentFor(option, locale)
  const sortedValues = useMemo(
    () => [...option.values].sort((a, b) => a.sortOrder - b.sortOrder),
    [option.values],
  )
  const showValues = isSelect(option.inputType)
  return (
    <div className="grid gap-3 p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <SlidersIcon className="size-4 text-muted-foreground" />
            <span className="font-medium">
              {content.title || <span className="text-muted-foreground">Untitled option</span>}
            </span>
            <Badge variant="outline" className="font-normal">{INPUT_TYPE_LABEL[option.inputType]}</Badge>
            {option.isRequired ? (
              <Badge variant="secondary" className="font-normal">Required</Badge>
            ) : (
              <Badge variant="outline" className="font-normal text-muted-foreground">Optional</Badge>
            )}
            {(parseVisibleIfGroup(option.visibleIf)?.rules?.length ?? 0) > 0 && (
              <Badge variant="outline" className="gap-1 font-normal text-muted-foreground">
                <EyeIcon className="size-3" />
                Conditional
              </Badge>
            )}
            {pending && (
              <Badge variant="outline" className="font-normal text-warning">Unsaved</Badge>
            )}
          </div>
          {content.helpText && (
            <p className="text-xs text-muted-foreground">{content.helpText}</p>
          )}
          {!showValues && option.priceValue !== null && option.priceType && (
            <p className="text-xs text-muted-foreground font-mono">
              {option.priceType === 'PERCENT' ? `+${option.priceValue}%` : `+${option.priceValue} ¢`}
            </p>
          )}
        </div>
        {canManage && (
          <div className="flex shrink-0 gap-1">
            <Button variant="ghost" size="icon" className="size-7" onClick={onDuplicate} aria-label="Duplicate option" disabled={pending}>
              <CopyIcon className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon" className="size-7" onClick={onEdit} aria-label="Edit option">
              <PencilIcon className="size-3.5" />
            </Button>
            <TwoClickDelete label="Delete option" onConfirm={onDelete} />
          </div>
        )}
      </div>

      {showValues && (
        <>
          {sortedValues.length === 0 ? (
            <p className="rounded-md border border-dashed px-4 py-4 text-center text-xs text-muted-foreground">
              No values yet. Add one to fill this option's menu.
            </p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Title</TableHead>
                    <TableHead className="w-32">Linked product</TableHead>
                    <TableHead className="w-16">Qty</TableHead>
                    <TableHead className="w-24">Line qty</TableHead>
                    <TableHead className="w-24">Price</TableHead>
                    <TableHead className="w-24">Type</TableHead>
                    <TableHead className="w-20">Default</TableHead>
                    {canManage && <TableHead className="w-20" />}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedValues.map((v) => {
                    const vp = isTemp(v.id)
                    return (
                      <TableRow key={v.id} className={vp ? 'bg-warning-soft' : ''}>
                        <TableCell className="font-medium">{valueContentFor(v, locale)}</TableCell>
                        <TableCell className="font-mono text-xs">
                          {v.sourceProduct ? (
                            <span className="inline-flex items-center gap-1">
                              <LinkIcon className="size-3" />
                              {v.sourceProduct.sku ?? ''}
                            </span>
                          ) : (
                            <span className="text-muted-foreground">-</span>
                          )}
                        </TableCell>
                        <TableCell className="font-mono text-xs">{v.quantity}</TableCell>
                        <TableCell className="font-mono text-xs">
                          {v.forcedLineQty != null ? `locks ×${v.forcedLineQty}` : '-'}
                        </TableCell>
                        <TableCell className="font-mono text-xs">{v.priceValue}</TableCell>
                        <TableCell className="font-mono text-xs">{v.priceType.toLowerCase()}</TableCell>
                        <TableCell>{v.isDefault ? '✓' : '-'}</TableCell>
                        {canManage && (
                          <TableCell>
                            <div className="flex gap-1">
                              <Button variant="ghost" size="icon" className="size-7" onClick={() => onEditValue(v)} aria-label="Edit value">
                                <PencilIcon className="size-3.5" />
                              </Button>
                              <TwoClickDelete label="Remove value" onConfirm={() => onDeleteValue(v)} />
                            </div>
                          </TableCell>
                        )}
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
          {canManage && !valueFormSlot && (
            <div className="flex justify-end">
              <Button variant="outline" size="sm" onClick={onAddValue}>
                <PlusIcon className="size-4" />
                Add value
              </Button>
            </div>
          )}
          {valueFormSlot}
        </>
      )}
    </div>
  )
}

// TwoClickDelete arms on the first click and stages on the second - deletes are staged (undone by leaving the page before the page-level
// save), so a blocking modal confirm is overkill; this keeps the safety beat without leaving the page flow - Disarms itself after 3s
function TwoClickDelete({ label, onConfirm }: { label: string; onConfirm: () => void }) {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const t = setTimeout(() => setArmed(false), 3000)
    return () => clearTimeout(t)
  }, [armed])
  if (armed) {
    return (
      <Button
        variant="destructive"
        size="sm"
        className="h-7 px-2 text-xs"
        onClick={onConfirm}
        aria-label={`Confirm: ${label}`}
      >
        Sure?
      </Button>
    )
  }
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive"
      onClick={() => setArmed(true)}
      aria-label={label}
    >
      <Trash2Icon className="size-3.5" />
    </Button>
  )
}

// ──────────────────────── Add option ────────────────────────

function AddOptionForm({
  locale,
  existingMaxSort,
  surcharges,
  onStage,
  onClose,
}: {
  locale: string
  existingMaxSort: number
  surcharges: Surcharges
  onStage: (op: PendingOp) => void
  onClose: () => void
}) {
  const [title, setTitle] = useState('')
  const [helpText, setHelpText] = useState('')
  const [inputType, setInputType] = useState<ProductOptionInputType>('FIELD')
  const [isRequired, setIsRequired] = useState(false)
  const [priceValue, setPriceValue] = useState<string>('')
  const [priceType, setPriceType] = useState<ProductOptionPriceType>('FIXED')
  const [maxChars, setMaxChars] = useState<string>('')
  const [maxFileBytes, setMaxFileBytes] = useState<string>('')
  const [fileExtensions, setFileExtensions] = useState<string>('')
  const [channelPrices, setChannelPrices] = useState<Record<string, string>>({})

  const showPriceOnOption = !isSelect(inputType)
  const showTextConstraints = isText(inputType)
  const showFileConstraints = isFile(inputType)
  const fixedSurcharge = showPriceOnOption && priceValue.trim() !== '' && priceType === 'FIXED'

  function reset() {
    setTitle('')
    setHelpText('')
    setInputType('FIELD')
    setIsRequired(false)
    setPriceValue('')
    setPriceType('FIXED')
    setMaxChars('')
    setMaxFileBytes('')
    setFileExtensions('')
    setChannelPrices({})
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const pv = priceValue.trim() === '' ? null : Number(priceValue)
    const priceEdits = fixedSurcharge ? channelEditsFrom(surcharges.channels, channelPrices, []) : undefined
    onStage({
      kind: 'add-option',
      tempId: newTempId(),
      inputType,
      isRequired,
      sortOrder: existingMaxSort + 1,
      title: title.trim(),
      helpText: helpText.trim(),
      locale,
      priceValue: showPriceOnOption ? pv : null,
      priceType: showPriceOnOption && pv !== null ? priceType : null,
      maxChars: showTextConstraints && maxChars.trim() !== '' ? Number(maxChars) : null,
      maxFileBytes: showFileConstraints && maxFileBytes.trim() !== '' ? Number(maxFileBytes) : null,
      fileExtensions: showFileConstraints && fileExtensions.trim() !== '' ? fileExtensions.trim() : null,
      ...(priceEdits ? { channelPrices: priceEdits } : {}),
    })
    reset()
    onClose()
  }

  return (
    <div className="grid gap-4 rounded-md border border-dashed bg-muted/20 p-4">
      <div className="grid gap-0.5">
        <h4 className="text-sm font-medium">New option</h4>
        <p className="text-xs text-muted-foreground">
          Customer-facing input for this product. Stages until the page-level save.
        </p>
      </div>

      <form id="add-product-option" onSubmit={onSubmit} className="grid gap-4 sm:max-w-md">
          <div className="grid gap-2">
            <Label htmlFor="po-title">Title</Label>
            <Input id="po-title" required placeholder="Engrave a name" value={title} onChange={(e) => setTitle(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              First-translation title in <code className="font-mono">{locale}</code>.
            </p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="po-help">Help text (optional)</Label>
            <Input id="po-help" placeholder="Letters and spaces only, up to 20 characters" value={helpText} onChange={(e) => setHelpText(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              Short hint rendered under the input on the channel.
            </p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="po-type">Input type</Label>
            <Select value={inputType} onValueChange={(v) => setInputType(v as ProductOptionInputType)}>
              <SelectTrigger id="po-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {INPUT_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>{INPUT_TYPE_LABEL[t]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-md border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="po-required" className="text-sm">Required</Label>
              <p className="text-xs text-muted-foreground">When on, the customer must fill this option to add to cart.</p>
            </div>
            <Switch id="po-required" checked={isRequired} onCheckedChange={setIsRequired} />
          </div>

          {showPriceOnOption && (
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-2">
                <Label htmlFor="po-price-value">Price adjustment</Label>
                <Input id="po-price-value" type="number" placeholder="0" value={priceValue} onChange={(e) => setPriceValue(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="po-price-type">Type</Label>
                <Select value={priceType} onValueChange={(v) => setPriceType(v as ProductOptionPriceType)}>
                  <SelectTrigger className="w-full" id="po-price-type"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="FIXED">Fixed (¢)</SelectItem>
                    <SelectItem value="PERCENT">Percent (%)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
          {fixedSurcharge && (
            <ChannelSurchargeInputs
              idPrefix="po-channel-price-add"
              channels={surcharges.channels}
              baseCurrency={surcharges.baseCurrency}
              baseAmount={Number(priceValue) || 0}
              values={channelPrices}
              onChange={(id, raw) => setChannelPrices((prev) => ({ ...prev, [id]: raw }))}
            />
          )}
          {isSelect(inputType) && (
            <p className="text-xs text-muted-foreground">
              Select-style options carry price on their values. Add values to this option after staging.
            </p>
          )}
          {showTextConstraints && (
            <div className="grid gap-2">
              <Label htmlFor="po-max-chars">Max characters (optional)</Label>
              <Input id="po-max-chars" type="number" min={1} value={maxChars} onChange={(e) => setMaxChars(e.target.value)} />
            </div>
          )}
          {showFileConstraints && (
            <>
              <div className="grid gap-2">
                <Label htmlFor="po-max-bytes">Max file size in bytes (optional)</Label>
                <Input id="po-max-bytes" type="number" min={1} value={maxFileBytes} onChange={(e) => setMaxFileBytes(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="po-extensions">Allowed extensions (optional)</Label>
                <Input id="po-extensions" placeholder="jpg,png,pdf" value={fileExtensions} onChange={(e) => setFileExtensions(e.target.value)} />
                <p className="text-xs text-muted-foreground">Comma-separated. Empty = any extension.</p>
              </div>
            </>
          )}
      </form>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onClose}>Cancel</Button>
        <Button type="submit" size="sm" form="add-product-option" disabled={!title.trim()}>Stage add</Button>
      </div>
    </div>
  )
}

// ──────────────────────── Edit option ────────────────────────

function EditOptionForm({
  option,
  siblings,
  locale,
  defaultLocale,
  productId,
  surcharges,
  onStage,
  onClose,
}: {
  option: ProductOption
  siblings: ProductOption[]
  locale: string
  defaultLocale: string
  productId: string
  surcharges: Surcharges
  onStage: (op: PendingOp) => void
  onClose: () => void
}) {
  const [deleteOptionContent] = useMutation(DeleteProductOptionContentDocument, {
    refetchQueries: [{ query: ProductDocument, variables: { id: productId } }],
  })
  const localeContent = optionContentFor(option, locale)
  const [title, setTitle] = useState(localeContent.title)
  const [helpText, setHelpText] = useState(localeContent.helpText)
  const [isRequired, setIsRequired] = useState(option.isRequired)
  const [sortOrder, setSortOrder] = useState(option.sortOrder)
  const [priceValue, setPriceValue] = useState<string>(option.priceValue === null ? '' : String(option.priceValue))
  const [priceType, setPriceType] = useState<ProductOptionPriceType>(option.priceType ?? 'FIXED')
  const [maxChars, setMaxChars] = useState<string>(option.maxChars === null ? '' : String(option.maxChars))
  const [maxFileBytes, setMaxFileBytes] = useState<string>(option.maxFileBytes === null ? '' : String(option.maxFileBytes))
  const [fileExtensions, setFileExtensions] = useState<string>(option.fileExtensions ?? '')
  const [channelPrices, setChannelPrices] = useState(() => rawChannelPrices(option.channelPrices))

  const showPriceOnOption = !isSelect(option.inputType)
  const showTextConstraints = isText(option.inputType)
  const showFileConstraints = isFile(option.inputType)
  // A translated title on an unsaved option would fold into the add op and
  // create it as the default-locale row - so block copy edits until saved
  const titleLocked = isTemp(option.id) && locale !== defaultLocale

  // Visibility condition (flat rules + AND/OR; nested groups are evaluator-only and can't be authored here) - Rules may only reference
  // options ordered strictly BEFORE this one - the backend enforces it, the parent Select simply doesn't offer anything else
  const currentVis = parseVisibleIfGroup(option.visibleIf)
  const [visLogic, setVisLogic] = useState<'and' | 'or'>(currentVis?.logic === 'or' ? 'or' : 'and')
  const [visRules, setVisRules] = useState<VisibilityRule[]>(currentVis?.rules ?? [])
  const hasNestedGroups = (currentVis?.groups?.length ?? 0) > 0
  const priorOptions = useMemo(
    () =>
      siblings.filter(
        (s) => s.id !== option.id && !isTemp(s.id) && s.sortOrder < sortOrder,
      ),
    [siblings, option.id, sortOrder],
  )
  const priorById = useMemo(() => new Map(priorOptions.map((o) => [o.id, o])), [priorOptions])
  const incompleteRule = visRules.some((r) => {
    if (!r.option_id || !priorById.has(r.option_id)) return true
    if (r.op === 'anyOf' || r.op === 'noneOf') return (r.value_ids?.length ?? 0) === 0
    return false
  })

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (incompleteRule) return
    const cleanRules: VisibilityRule[] = visRules.map((r) =>
      r.op === 'anyOf' || r.op === 'noneOf'
        ? { option_id: r.option_id, op: r.op, value_ids: r.value_ids }
        : { option_id: r.option_id, op: r.op },
    )
    const nextVisibleIf: VisibilityGroup | null =
      cleanRules.length === 0
        ? null
        : { ...(visLogic === 'or' ? { logic: 'or' as const } : {}), rules: cleanRules }
    const visChanged =
      JSON.stringify(nextVisibleIf) !== JSON.stringify(currentVis ?? null)
    const pv = priceValue.trim() === '' ? null : Number(priceValue)
    // Title + help text are one per-locale unit: when either changes, send
    // both (plus the locale) so the content upsert replaces the row intact
    const contentChanged =
      !titleLocked && (title.trim() !== localeContent.title || helpText !== localeContent.helpText)
    const op = {
      kind: 'update-option' as const,
      id: option.id,
      ...(contentChanged ? { title: title.trim(), helpText, titleLocale: locale } : {}),
      ...(isRequired !== option.isRequired ? { isRequired } : {}),
      ...(sortOrder !== option.sortOrder ? { sortOrder } : {}),
      ...(showPriceOnOption && pv !== option.priceValue ? { priceValue: pv } : {}),
      ...(showPriceOnOption && pv !== null && priceType !== option.priceType ? { priceType } : {}),
      ...(showTextConstraints
        ? maxChars === '' && option.maxChars !== null
          ? { maxChars: null }
          : maxChars !== '' && Number(maxChars) !== option.maxChars
            ? { maxChars: Number(maxChars) }
            : {}
        : {}),
      ...(showFileConstraints
        ? maxFileBytes === '' && option.maxFileBytes !== null
          ? { maxFileBytes: null }
          : maxFileBytes !== '' && Number(maxFileBytes) !== option.maxFileBytes
            ? { maxFileBytes: Number(maxFileBytes) }
            : {}
        : {}),
      ...(showFileConstraints && fileExtensions !== (option.fileExtensions ?? '')
        ? { fileExtensions: fileExtensions.trim() === '' ? null : fileExtensions.trim() }
        : {}),
      ...(visChanged && !hasNestedGroups ? { visibleIf: nextVisibleIf } : {}),
    }
    const priceEdits =
      showPriceOnOption && pv !== null && priceType === 'FIXED'
        ? channelEditsFrom(surcharges.channels, channelPrices, option.channelPrices)
        : undefined
    if (priceEdits) Object.assign(op, { channelPrices: priceEdits })
    if (Object.keys(op).length > 2) onStage(op)
    onClose()
  }

  return (
    <div className="grid gap-4 border-l-2 border-l-primary bg-muted/20 p-4">
      <div className="grid gap-0.5">
        <h4 className="text-sm font-medium">
          Editing <span className="text-primary">{localeContent.title || 'Untitled option'}</span>
          <span className="ml-1.5 font-normal text-muted-foreground">· {INPUT_TYPE_LABEL[option.inputType]}</span>
        </h4>
      </div>

      <form id="edit-product-option" onSubmit={onSubmit} className="grid gap-4 sm:max-w-xl">
          <div className="grid gap-2">
            <Label htmlFor="po-title-edit">Title ({locale})</Label>
            <Input id="po-title-edit" required value={title} onChange={(e) => setTitle(e.target.value)} disabled={titleLocked} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="po-help-edit">Help text ({locale})</Label>
            <Input id="po-help-edit" placeholder="(optional)" value={helpText} onChange={(e) => setHelpText(e.target.value)} disabled={titleLocked} />
            {titleLocked && (
              <p className="text-xs text-muted-foreground">
                Unsaved option - save it first, then switch the edit locale to translate.
              </p>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex items-start justify-between gap-4 rounded-md border p-3 col-span-2">
              <div className="grid gap-0.5">
                <Label htmlFor="po-required-edit" className="text-sm">Required</Label>
                <p className="text-xs text-muted-foreground">Customer must fill this option to add to cart.</p>
              </div>
              <Switch id="po-required-edit" checked={isRequired} onCheckedChange={setIsRequired} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="po-sort-edit">Sort order</Label>
              <Input id="po-sort-edit" type="number" inputMode="numeric" value={sortOrder} onChange={(e) => setSortOrder(Number(e.target.value) || 0)} />
            </div>
          </div>
          {showPriceOnOption && (
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-2">
                <Label htmlFor="po-price-value-edit">Price adjustment</Label>
                <Input id="po-price-value-edit" type="number" placeholder="(none)" value={priceValue} onChange={(e) => setPriceValue(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="po-price-type-edit">Type</Label>
                <Select value={priceType} onValueChange={(v) => setPriceType(v as ProductOptionPriceType)}>
                  <SelectTrigger className="w-full" id="po-price-type-edit"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="FIXED">Fixed (¢)</SelectItem>
                    <SelectItem value="PERCENT">Percent (%)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
          {showPriceOnOption && priceValue.trim() !== '' && priceType === 'FIXED' && (
            <ChannelSurchargeInputs
              idPrefix="po-channel-price"
              channels={surcharges.channels}
              baseCurrency={surcharges.baseCurrency}
              baseAmount={Number(priceValue) || 0}
              values={channelPrices}
              onChange={(id, raw) => setChannelPrices((prev) => ({ ...prev, [id]: raw }))}
            />
          )}
          {showTextConstraints && (
            <div className="grid gap-2">
              <Label htmlFor="po-max-chars-edit">Max characters (optional)</Label>
              <Input id="po-max-chars-edit" type="number" min={1} value={maxChars} onChange={(e) => setMaxChars(e.target.value)} />
            </div>
          )}
          {showFileConstraints && (
            <>
              <div className="grid gap-2">
                <Label htmlFor="po-max-bytes-edit">Max file size in bytes (optional)</Label>
                <Input id="po-max-bytes-edit" type="number" min={1} value={maxFileBytes} onChange={(e) => setMaxFileBytes(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="po-extensions-edit">Allowed extensions (optional)</Label>
                <Input id="po-extensions-edit" placeholder="jpg,png,pdf" value={fileExtensions} onChange={(e) => setFileExtensions(e.target.value)} />
              </div>
            </>
          )}

          <VisibilityEditor
            optionIsTemp={isTemp(option.id)}
            hasNestedGroups={hasNestedGroups}
            priorOptions={priorOptions}
            locale={locale}
            logic={visLogic}
            rules={visRules}
            onLogicChange={setVisLogic}
            onRulesChange={setVisRules}
          />
      </form>

      {/* Title + help text above are per-locale - Dropping a locale is
          immediate (not staged) - it's destructive and the staged diff has
          no way to express "delete this translation" - Temp options have no
          server row yet, so there's nothing to delete */}
      {!isTemp(option.id) && (
        <TranslationLocales
          locale={locale}
          locales={(option.content ?? []).map((c) => c.locale)}
          entityLabel="option"
          canManage
          defaultLocaleLocked
          onRemove={async () => {
            await deleteOptionContent({ variables: { optionId: option.id, locale } })
          }}
        />
      )}

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onClose}>Cancel</Button>
        <Button type="submit" size="sm" form="edit-product-option" disabled={incompleteRule}>Stage edit</Button>
      </div>
    </div>
  )
}

// ──────────────────────── Visibility condition editor ────────────────────────
//
// Flat rule list + AND/OR toggle over PRIOR options only (strictly lower
// sort order - the backend's ordering invariant, which is what makes cart
// evaluation a single cycle-free pass). anyOf/noneOf target select-family
// parents and pick from that parent's value list; filled/empty work on any
// parent - Mirrors the webform ConditionEditor, but value-id based

const VIS_OP_LABEL: Record<VisibilityRule['op'], string> = {
  anyOf: 'has any of',
  noneOf: 'has none of',
  filled: 'is filled',
  empty: 'is empty',
}

function VisibilityEditor({
  optionIsTemp,
  hasNestedGroups,
  priorOptions,
  locale,
  logic,
  rules,
  onLogicChange,
  onRulesChange,
}: {
  optionIsTemp: boolean
  hasNestedGroups: boolean
  priorOptions: ProductOption[]
  locale: string
  logic: 'and' | 'or'
  rules: VisibilityRule[]
  onLogicChange: (l: 'and' | 'or') => void
  onRulesChange: (r: VisibilityRule[]) => void
}) {
  if (optionIsTemp) {
    return (
      <div className="grid gap-1 rounded-md border border-dashed p-3">
        <p className="text-sm font-medium">Visibility</p>
        <p className="text-xs text-muted-foreground">
          Save the new option first, then add rules that show it based on earlier options.
        </p>
      </div>
    )
  }
  if (hasNestedGroups) {
    return (
      <div className="grid gap-1 rounded-md border border-dashed p-3">
        <p className="text-sm font-medium">Visibility</p>
        <p className="text-xs text-muted-foreground">
          This option uses nested visibility groups (authored via the API), which this editor
          can't represent. Edit the condition via the API to change it.
        </p>
      </div>
    )
  }

  const update = (i: number, patch: Partial<VisibilityRule>) => {
    onRulesChange(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  }

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="grid gap-0.5">
          <p className="text-sm font-medium">Visibility</p>
          <p className="text-xs text-muted-foreground">
            {rules.length === 0
              ? 'Always shown. Add a rule to show this option only for certain earlier choices.'
              : 'Shown when the rules below match. Hidden options are never required and never priced in.'}
          </p>
        </div>
        {rules.length > 1 && (
          <Select value={logic} onValueChange={(v) => onLogicChange(v as 'and' | 'or')}>
            <SelectTrigger size="sm" className="w-[7.5rem]" aria-label="Rule logic">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="and">Match all</SelectItem>
              <SelectItem value="or">Match any</SelectItem>
            </SelectContent>
          </Select>
        )}
      </div>

      {rules.map((rule, i) => {
        const parent = priorOptions.find((o) => o.id === rule.option_id)
        const parentIsSelect = parent ? isSelect(parent.inputType) : false
        const ops: VisibilityRule['op'][] = parentIsSelect
          ? ['anyOf', 'noneOf', 'filled', 'empty']
          : ['filled', 'empty']
        const needsValues = rule.op === 'anyOf' || rule.op === 'noneOf'
        const sortedParentValues = parent
          ? [...parent.values].sort((a, b) => a.sortOrder - b.sortOrder)
          : []
        return (
          <div key={i} className="grid gap-2 rounded-md border bg-muted/30 p-2">
            <div className="flex items-center gap-2">
              <Select
                value={rule.option_id || undefined}
                onValueChange={(v) => {
                  const nextParent = priorOptions.find((o) => o.id === v)
                  const nextIsSelect = nextParent ? isSelect(nextParent.inputType) : false
                  update(i, {
                    option_id: v,
                    // Reset the op when it stops making sense for the new parent
                    op: !nextIsSelect && (rule.op === 'anyOf' || rule.op === 'noneOf') ? 'filled' : rule.op,
                    value_ids: [],
                  })
                }}
              >
                <SelectTrigger size="sm" className="flex-1" aria-label="Depends on option">
                  <SelectValue placeholder="Pick an earlier option…" />
                </SelectTrigger>
                <SelectContent>
                  {priorOptions.map((o) => (
                    <SelectItem key={o.id} value={o.id}>
                      {optionContentFor(o, locale).title || 'Untitled option'}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={rule.op} onValueChange={(v) => update(i, { op: v as VisibilityRule['op'], value_ids: [] })}>
                <SelectTrigger size="sm" className="w-[8.5rem]" aria-label="Condition">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ops.map((o) => (
                    <SelectItem key={o} value={o}>{VIS_OP_LABEL[o]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-7 shrink-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => onRulesChange(rules.filter((_, j) => j !== i))}
                aria-label="Remove rule"
              >
                <Trash2Icon className="size-3.5" />
              </Button>
            </div>
            {needsValues && parent && (
              sortedParentValues.length === 0 ? (
                <p className="text-xs text-destructive">That option has no values yet - add values to it first.</p>
              ) : (
                <div className="flex flex-wrap gap-x-4 gap-y-1.5 px-1">
                  {sortedParentValues.map((v) => {
                    const checked = rule.value_ids?.includes(v.id) ?? false
                    return (
                      <label key={v.id} className="flex items-center gap-1.5 text-sm">
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(c) => {
                            const cur = rule.value_ids ?? []
                            update(i, {
                              value_ids: c === true ? [...cur, v.id] : cur.filter((id) => id !== v.id),
                            })
                          }}
                        />
                        {valueContentFor(v, locale)}
                      </label>
                    )
                  })}
                </div>
              )
            )}
            {needsValues && (rule.value_ids?.length ?? 0) === 0 && sortedParentValues.length > 0 && (
              <p className="text-xs text-muted-foreground">Pick at least one value.</p>
            )}
          </div>
        )
      })}

      {priorOptions.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          No earlier options to depend on. Rules can only reference options ordered before this
          one - adjust sort orders first if needed.
        </p>
      ) : (
        <div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onRulesChange([...rules, { option_id: '', op: 'filled' }])}
          >
            <PlusIcon className="size-4" />
            Add rule
          </Button>
        </div>
      )}
    </div>
  )
}

// ──────────────────────── Source-product picker ────────────────────────
//
// Inline picker used by Add/Edit value dialogs - Renders either the
// currently-linked product as a chip with a clear button, or a search
// input + scrollable result list - SIMPLE products only; the option
// value's own product is excluded from the candidate list to prevent
// trivial self-reference (the catalog still hard-rejects it)

function SourceProductPicker({
  value,
  excludeProductId,
  onChange,
}: {
  value: SourceProduct | null
  excludeProductId?: string
  onChange: (next: SourceProduct | null) => void
}) {
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const { data, loading } = useQuery(ProductPickerDocument, {
    variables: { search: debounced || null, kinds: ['SIMPLE'], limit: 12 },
  })
  // the exclusion stays local - the page is one product-detail screen, so at most one row drops
  const filtered = useMemo(() => {
    const all = (data?.productsPaged.items ?? []) as Array<SourceProduct>
    return all.filter((p) => (excludeProductId ? p.id !== excludeProductId : true)).slice(0, 8)
  }, [data, excludeProductId])

  if (value) {
    return (
      <div className="flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2">
        <div className="grid gap-0.5 text-sm">
          <span className="font-medium">{value.name ?? value.sku ?? ''}</span>
          {value.sku && <code className="font-mono text-xs text-muted-foreground">{value.sku}</code>}
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="size-7"
          onClick={() => onChange(null)}
          aria-label="Unlink product"
          type="button"
        >
          <XIcon className="size-3.5" />
        </Button>
      </div>
    )
  }

  return (
    <div className="grid gap-2">
      <Input
        type="search"
        placeholder={loading ? 'Loading products…' : 'Search to link a product…'}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="h-8"
      />
      {search.trim() !== '' && (
        <div className="grid gap-1 rounded-md border bg-popover p-1 text-sm">
          {filtered.length === 0 ? (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">No matches.</p>
          ) : (
            filtered.map((p) => (
              <button
                key={p.id}
                type="button"
                className="grid gap-0.5 rounded px-2 py-1.5 text-left hover:bg-accent"
                onClick={() => {
                  onChange(p)
                  setSearch('')
                }}
              >
                <span className="font-medium">{p.name ?? p.sku ?? ''}</span>
                <span className="font-mono text-xs text-muted-foreground">{p.sku ?? ''}</span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}

// ──────────────────────── Add value ────────────────────────

function AddValueForm({
  option,
  locale,
  surcharges,
  onStage,
  onClose,
}: {
  option: ProductOption
  locale: string
  surcharges: Surcharges
  onStage: (op: PendingOp) => void
  onClose: () => void
}) {
  const [title, setTitle] = useState('')
  const [priceValue, setPriceValue] = useState('0')
  const [priceType, setPriceType] = useState<ProductOptionPriceType>('FIXED')
  const [isDefault, setIsDefault] = useState(false)
  const [quantity, setQuantity] = useState('1')
  const [forcedLineQty, setForcedLineQty] = useState('')
  const [sourceProduct, setSourceProduct] = useState<SourceProduct | null>(null)
  const [channelPrices, setChannelPrices] = useState<Record<string, string>>({})
  const fixed = priceType === 'FIXED' || !!sourceProduct

  function reset() {
    setTitle('')
    setPriceValue('0')
    setPriceType('FIXED')
    setIsDefault(false)
    setQuantity('1')
    setForcedLineQty('')
    setSourceProduct(null)
    setChannelPrices({})
  }

  function onPickProduct(next: SourceProduct | null) {
    setSourceProduct(next)
    if (next) {
      // Auto-fill title with the product's name (admin can edit) and
      // flip to FIXED price (PERCENT isn't allowed on linked values)
      if (!title.trim()) setTitle(next.name ?? next.sku ?? '')
      setPriceType('FIXED')
    }
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const nextSort = option.values.reduce((m, v) => Math.max(m, v.sortOrder), -1) + 1
    onStage({
      kind: 'add-value',
      tempId: newTempId(),
      optionRef: option.id,
      sortOrder: nextSort,
      priceValue: Number(priceValue) || 0,
      priceType,
      isDefault,
      quantity: Math.max(1, Number(quantity) || 1),
      forcedLineQty:
        forcedLineQty.trim() !== '' && Math.trunc(Number(forcedLineQty)) > 0 ? Math.trunc(Number(forcedLineQty)) : null,
      title: title.trim(),
      locale,
      sourceProductId: sourceProduct?.id ?? null,
      sourceProduct,
      ...(fixed ? { channelPrices: channelEditsFrom(surcharges.channels, channelPrices, []) } : {}),
    })
    reset()
    onClose()
  }

  return (
    <div className="grid gap-4 border-l-2 border-l-primary bg-muted/20 p-3">
      <div className="grid gap-0.5">
        <h4 className="text-sm font-medium">New value</h4>
        <p className="text-xs text-muted-foreground">One choice the customer can pick. Stages until the page-level save.</p>
      </div>
      <form id="add-product-option-value" onSubmit={onSubmit} className="grid gap-4 sm:max-w-md">
          <div className="grid gap-2">
            <Label>Linked product (optional)</Label>
            <SourceProductPicker value={sourceProduct} onChange={onPickProduct} />
            <p className="text-xs text-muted-foreground">
              Linking inherits the product's SKU + base price; the price adjustment below
              becomes a markup on top.
            </p>
            {sourceProduct && (
              <p className="text-xs text-muted-foreground">
                Each pick also takes the linked product's stock, this value's quantity per unit ordered.
              </p>
            )}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pov-title">Title ({locale})</Label>
            <Input id="pov-title" required value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3 items-start">
            <div className="grid gap-2">
              <Label htmlFor="pov-price">{sourceProduct ? 'Markup' : 'Price adjustment'}</Label>
              <Input id="pov-price" type="number" value={priceValue} onChange={(e) => setPriceValue(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pov-type">Type</Label>
              <Select
                value={priceType}
                onValueChange={(v) => setPriceType(v as ProductOptionPriceType)}
                disabled={!!sourceProduct}
              >
                <SelectTrigger className="w-full" id="pov-type"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="FIXED">Fixed (¢)</SelectItem>
                  <SelectItem value="PERCENT" disabled={!!sourceProduct}>Percent (%)</SelectItem>
                </SelectContent>
              </Select>
              {sourceProduct && (
                <p className="text-xs text-muted-foreground">PERCENT not allowed on linked values.</p>
              )}
            </div>
          </div>
          {fixed && (
            <ChannelSurchargeInputs
              idPrefix="pov-channel-price"
              channels={surcharges.channels}
              baseCurrency={surcharges.baseCurrency}
              baseAmount={Number(priceValue) || 0}
              values={channelPrices}
              onChange={(id, raw) => setChannelPrices((prev) => ({ ...prev, [id]: raw }))}
            />
          )}
          <div className="grid gap-2">
            <Label htmlFor="pov-quantity">Quantity</Label>
            <Input id="pov-quantity" type="number" min={1} value={quantity} onChange={(e) => setQuantity(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              Units the value represents (default 1). {sourceProduct ? "Multiplied against the linked product's price at cart time." : 'Metadata for now; cart pricing applies it when product options ship in PR 2.'}
            </p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pov-forced-qty">Forced line quantity (optional)</Label>
            <Input id="pov-forced-qty" type="number" min={1} step={1} placeholder="(none)" value={forcedLineQty} onChange={(e) => setForcedLineQty(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              Picking this value sets the cart line quantity to exactly this amount and locks
              it ("Set of 4" → 4). Leave empty for no constraint.
            </p>
          </div>
          <div className="flex items-start justify-between gap-4 rounded-md border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="pov-default" className="text-sm">Default</Label>
              <p className="text-xs text-muted-foreground">
                Pre-selected on the channel. {isSingleSelect(option.inputType) ? 'Single-select: only one default allowed.' : 'Multi-select: any number of defaults allowed.'}
              </p>
            </div>
            <Switch id="pov-default" checked={isDefault} onCheckedChange={setIsDefault} />
          </div>
      </form>
      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onClose}>Cancel</Button>
        <Button type="submit" size="sm" form="add-product-option-value" disabled={!title.trim()}>Stage add</Button>
      </div>
    </div>
  )
}

// ──────────────────────── Edit value ────────────────────────

function EditValueForm({
  value,
  locale,
  defaultLocale,
  productId,
  surcharges,
  onStage,
  onClose,
}: {
  value: OptionValue
  locale: string
  defaultLocale: string
  productId: string
  surcharges: Surcharges
  onStage: (op: PendingOp) => void
  onClose: () => void
}) {
  const [deleteValueContent] = useMutation(DeleteProductOptionValueContentDocument, {
    refetchQueries: [{ query: ProductDocument, variables: { id: productId } }],
  })
  const localeTitle = valueContentFor(value, locale)
  const [title, setTitle] = useState(localeTitle)
  const [priceValue, setPriceValue] = useState(String(value.priceValue))
  const [priceType, setPriceType] = useState<ProductOptionPriceType>(value.priceType)
  const [isDefault, setIsDefault] = useState(value.isDefault)
  const [sortOrder, setSortOrder] = useState(value.sortOrder)
  const [quantity, setQuantity] = useState(String(value.quantity))
  const [forcedLineQty, setForcedLineQty] = useState(value.forcedLineQty != null ? String(value.forcedLineQty) : '')
  const [sourceProduct, setSourceProduct] = useState<SourceProduct | null>(value.sourceProduct ?? null)
  const [channelPrices, setChannelPrices] = useState(() => rawChannelPrices(value.channelPrices))
  const fixed = priceType === 'FIXED' || !!sourceProduct
  // same fold hazard as option titles - see EditOptionForm
  const titleLocked = isTemp(value.id) && locale !== defaultLocale

  function onPickProduct(next: SourceProduct | null) {
    setSourceProduct(next)
    if (next) setPriceType('FIXED')
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const pv = Number(priceValue) || 0
    const qty = Math.max(1, Number(quantity) || 1)
    const nextForced =
      forcedLineQty.trim() !== '' && Math.trunc(Number(forcedLineQty)) > 0 ? Math.trunc(Number(forcedLineQty)) : null
    const prevSrcId = value.sourceProduct?.id ?? null
    const nextSrcId = sourceProduct?.id ?? null
    const op = {
      kind: 'update-value' as const,
      id: value.id,
      ...(!titleLocked && title.trim() !== localeTitle ? { title: title.trim(), titleLocale: locale } : {}),
      ...(pv !== value.priceValue ? { priceValue: pv } : {}),
      ...(priceType !== value.priceType ? { priceType } : {}),
      ...(isDefault !== value.isDefault ? { isDefault } : {}),
      ...(sortOrder !== value.sortOrder ? { sortOrder } : {}),
      ...(nextForced !== (value.forcedLineQty ?? null) ? { forcedLineQty: nextForced } : {}),
      ...(qty !== value.quantity ? { quantity: qty } : {}),
      ...(nextSrcId !== prevSrcId
        ? { sourceProductId: nextSrcId, sourceProduct: sourceProduct ?? null }
        : {}),
    }
    const priceEdits = fixed ? channelEditsFrom(surcharges.channels, channelPrices, value.channelPrices) : undefined
    if (priceEdits) Object.assign(op, { channelPrices: priceEdits })
    if (Object.keys(op).length > 2) onStage(op)
    onClose()
  }

  return (
    <div className="grid gap-4 border-l-2 border-l-primary bg-muted/20 p-3">
      <div className="grid gap-0.5">
        <h4 className="text-sm font-medium">
          Editing value <span className="text-primary">{valueContentFor(value, locale) || 'Untitled'}</span>
        </h4>
      </div>
      <form id="edit-product-option-value" onSubmit={onSubmit} className="grid gap-4 sm:max-w-md">
          <div className="grid gap-2">
            <Label>Linked product (optional)</Label>
            <SourceProductPicker value={sourceProduct} onChange={onPickProduct} />
            {sourceProduct && (
              <p className="text-xs text-muted-foreground">
                Each pick also takes the linked product's stock, this value's quantity per unit ordered.
              </p>
            )}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pov-title-edit">Title ({locale})</Label>
            <Input id="pov-title-edit" required value={title} onChange={(e) => setTitle(e.target.value)} disabled={titleLocked} />
            {titleLocked && (
              <p className="text-xs text-muted-foreground">
                Unsaved value - save it first, then switch the edit locale to translate.
              </p>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="pov-price-edit">{sourceProduct ? 'Markup' : 'Price adjustment'}</Label>
              <Input id="pov-price-edit" type="number" value={priceValue} onChange={(e) => setPriceValue(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pov-type-edit">Type</Label>
              <Select
                value={priceType}
                onValueChange={(v) => setPriceType(v as ProductOptionPriceType)}
                disabled={!!sourceProduct}
              >
                <SelectTrigger className="w-full" id="pov-type-edit"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="FIXED">Fixed (¢)</SelectItem>
                  <SelectItem value="PERCENT" disabled={!!sourceProduct}>Percent (%)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          {fixed && (
            <ChannelSurchargeInputs
              idPrefix="pov-channel-price-edit"
              channels={surcharges.channels}
              baseCurrency={surcharges.baseCurrency}
              baseAmount={Number(priceValue) || 0}
              values={channelPrices}
              onChange={(id, raw) => setChannelPrices((prev) => ({ ...prev, [id]: raw }))}
            />
          )}
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="pov-sort-edit">Sort order</Label>
              <Input id="pov-sort-edit" type="number" inputMode="numeric" value={sortOrder} onChange={(e) => setSortOrder(Number(e.target.value) || 0)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pov-qty-edit">Quantity</Label>
              <Input id="pov-qty-edit" type="number" min={1} value={quantity} onChange={(e) => setQuantity(e.target.value)} />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pov-forced-qty-edit">Forced line quantity (optional)</Label>
            <Input id="pov-forced-qty-edit" type="number" min={1} step={1} placeholder="(none)" value={forcedLineQty} onChange={(e) => setForcedLineQty(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              Picking this value sets + locks the cart line quantity. Empty = no constraint.
            </p>
          </div>
          <div className="flex items-start justify-between gap-4 rounded-md border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="pov-default-edit" className="text-sm">Default</Label>
              <p className="text-xs text-muted-foreground">Pre-selected on the channel.</p>
            </div>
            <Switch id="pov-default-edit" checked={isDefault} onCheckedChange={setIsDefault} />
          </div>
      </form>
      {!isTemp(value.id) && (
        <TranslationLocales
          locale={locale}
          locales={(value.content ?? []).map((c) => c.locale)}
          entityLabel="option value"
          canManage
          defaultLocaleLocked
          onRemove={async () => {
            await deleteValueContent({ variables: { valueId: value.id, locale } })
          }}
        />
      )}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onClose}>Cancel</Button>
        <Button type="submit" size="sm" form="edit-product-option-value">Stage edit</Button>
      </div>
    </div>
  )
}
