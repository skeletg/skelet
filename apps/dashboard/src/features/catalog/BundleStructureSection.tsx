import { useCallback, useMemo, useState, type FormEvent } from 'react'
import { useMutation } from '@apollo/client/react'
import {
  AlertTriangleIcon,
  CheckCircle2Icon,
  PackagePlusIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { formatCents } from '@/lib/format'
import type { BundleInputType, BundlePricing, UpdateBundleOptionInput } from '@/graphql/schema.types'
import {
  AddBundleOptionDocument,
  AddBundleSelectionDocument,
  DeleteBundleOptionDocument,
  DeleteBundleSelectionDocument,
  ProductDocument,
  UpdateBundleOptionDocument,
  UpdateBundleSelectionDocument,
  DeleteBundleOptionContentDocument,
  SetBundlePricingDocument,
  UpsertBundleOptionContentDocument,
} from './Catalog.gql'
import {
  applyOps,
  CommitError,
  commitOps,
  isTemp,
  newTempId,
  opsToDiff,
  reduceOps,
  selectionLimitText,
  type BundleOption,
  type Committers,
  type PendingOp,
  type Selection,
  type SelectionProduct,
} from './bundleStaging'
import { storeHealth, structureIssues, type BundleListing, type HealthIssue, type StorePricing } from './bundleHealth'
import { useProductSaveSection } from './productSaveRegistry'
import { ProductPickerDialog, type PickExtra, type PickerProduct } from './ProductPickerDialog'
import { defaultColumn, quantityColumn } from './product-picker-columns'
import { TranslationLocales } from '@/components/TranslationLocales'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type Bundle = {
  id: string
  contentLocale?: string | null
  bundleOptions: BundleOption[]
  listings?: BundleListing[]
}

// Said as what the customer does - the widget name alone hid that checkbox and multiselect behave the same
const INPUT_TYPE_LABEL: Record<BundleInputType, string> = {
  RADIO: 'Pick one · radio',
  DROPDOWN: 'Pick one · dropdown',
  CHECKBOX: 'Pick several · checkboxes',
  MULTISELECT: 'Pick several · list',
}
const INPUT_TYPES: BundleInputType[] = ['RADIO', 'DROPDOWN', 'CHECKBOX', 'MULTISELECT']
const SINGLE_SELECT_TYPES: BundleInputType[] = ['RADIO', 'DROPDOWN']

function isSingleSelect(t: BundleInputType) {
  return SINGLE_SELECT_TYPES.includes(t)
}

type Limits = { min: number; max: number | null }
const NO_LIMITS: Limits = { min: 0, max: null }

// The backend refuses counts above one on a single-select option, so switching to one drops the limits
function effectiveLimits(t: BundleInputType, l: Limits): Limits {
  return isSingleSelect(t) ? NO_LIMITS : l
}

// The same bounds the database enforces, said before the save rather than after it
function limitsError({ min, max }: Limits): string | null {
  if (!Number.isInteger(min) || min < 0) return 'At least must be 0 or more.'
  if (max === null) return null
  if (!Number.isInteger(max) || max < 1) return 'At most must be 1 or more, or left empty for no limit.'
  if (max < min) return 'At most can’t be lower than at least.'
  return null
}

function SelectionLimitsFields({
  idPrefix,
  value,
  onChange,
  error,
  selectionCount,
}: {
  idPrefix: string
  value: Limits
  onChange: (l: Limits) => void
  error: string | null
  selectionCount?: number
}) {
  const shortBy = selectionCount !== undefined && value.min > selectionCount
  return (
    <div className="grid gap-2 rounded-md border p-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-2">
          <Label htmlFor={`${idPrefix}-min`}>At least</Label>
          <Input
            id={`${idPrefix}-min`}
            type="number"
            inputMode="numeric"
            min={0}
            value={value.min}
            onChange={(e) => onChange({ ...value, min: Number(e.target.value) || 0 })}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor={`${idPrefix}-max`}>At most</Label>
          <Input
            id={`${idPrefix}-max`}
            type="number"
            inputMode="numeric"
            min={1}
            placeholder="No limit"
            value={value.max ?? ''}
            onChange={(e) => onChange({ ...value, max: e.target.value === '' ? null : Number(e.target.value) })}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        How many items the customer picks here. 0 and empty mean no limit - a required option still needs one.
      </p>
      {error && <p className="text-xs text-destructive">{error}</p>}
      {!error && shortBy && (
        <p className="text-xs text-warning">
          Only {selectionCount} selection(s) so far - customers can’t pick {value.min} until you add more.
        </p>
      )}
    </div>
  )
}

export function BundleStructureSection({
  product,
  canManage,
}: {
  product: Bundle
  canManage: boolean
}) {
  // Pending ops collected from sub-dialogs - The section never fires mutations directly any more - every change goes
  // through this queue and is committed by the product page's global save
  const [pendingOps, setPendingOps] = useState<PendingOp[]>([])

  const stage = useCallback((op: PendingOp) => {
    setPendingOps((prev) => reduceOps([...prev, op]))
  }, [])

  const reduced = useMemo(() => reduceOps(pendingOps), [pendingOps])
  const staged = useMemo(() => applyOps(product.bundleOptions, reduced), [product.bundleOptions, reduced])
  const diff = useMemo(() => opsToDiff(product.bundleOptions, reduced), [product.bundleOptions, reduced])

  // Apollo hooks live up here so the save() closure can call them refetchQueries fires the product page's main query so
  // the new server state replaces the staged view after a successful commit
  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [addOptionMutation] = useMutation(AddBundleOptionDocument, { refetchQueries: refetch })
  const [updateOptionMutation] = useMutation(UpdateBundleOptionDocument, { refetchQueries: refetch })
  const [upsertOptionContentMutation] = useMutation(UpsertBundleOptionContentDocument, { refetchQueries: refetch })
  const [deleteOptionMutation] = useMutation(DeleteBundleOptionDocument, { refetchQueries: refetch })
  const [addSelectionMutation] = useMutation(AddBundleSelectionDocument, { refetchQueries: refetch })
  const [updateSelectionMutation] = useMutation(UpdateBundleSelectionDocument, { refetchQueries: refetch })
  const [deleteSelectionMutation] = useMutation(DeleteBundleSelectionDocument, { refetchQueries: refetch })
  const [deleteOptionContentMutation] = useMutation(DeleteBundleOptionContentDocument, { refetchQueries: refetch })

  const save = useCallback(async () => {
    if (reduced.length === 0) return
    // Track ids the server has acked so subsequent add lookups can
    // pick the truly-new row when multiple adds share a title
    const knownOptionIds = new Set(product.bundleOptions.map((o) => o.id))
    const committers: Committers = {
      addOption: async (op) => {
        const res = await addOptionMutation({
          variables: {
            input: {
              productId: op.productId,
              inputType: op.inputType,
              isRequired: op.isRequired,
              minSelections: op.minSelections,
              ...(op.maxSelections !== null ? { maxSelections: op.maxSelections } : {}),
              locale: op.locale,
              title: op.title,
            },
          },
        })
        // The mutation returns the parent Product with all bundle options re-projected - The just-created row is
        // whichever option's id we haven't seen yet - robust to identical titles being added back-to-back in one save
        const opts = res.data?.addBundleOption.bundleOptions ?? []
        const fresh = opts.find((o) => !knownOptionIds.has(o.id))
        if (!fresh) {
          throw new Error('Could not resolve newly-added bundle option id from response.')
        }
        knownOptionIds.add(fresh.id)
        return fresh.id
      },
      updateOption: async (op) => {
        const input: UpdateBundleOptionInput = { id: op.id }
        if (op.inputType !== undefined) input.inputType = op.inputType
        if (op.isRequired !== undefined) input.isRequired = op.isRequired
        if (op.sortOrder !== undefined) input.sortOrder = op.sortOrder
        if (op.minSelections !== undefined) input.minSelections = op.minSelections
        if (op.maxSelections === null) input.clearMaxSelections = true
        else if (op.maxSelections !== undefined) input.maxSelections = op.maxSelections
        // updateBundleOption only takes structural fields; title is
        // upserted separately by upsertOptionContent below
        if (Object.keys(input).length > 1) {
          await updateOptionMutation({ variables: { input } })
        }
      },
      upsertOptionContent: async (op) => {
        await upsertOptionContentMutation({
          variables: { optionId: op.optionId, locale: op.locale, title: op.title },
        })
      },
      deleteOption: async (op) => {
        await deleteOptionMutation({ variables: { id: op.id } })
      },
      deleteOptionContent: async (op) => {
        await deleteOptionContentMutation({ variables: { optionId: op.optionId, locale: op.locale } })
      },
      addSelection: async (op) => {
        const res = await addSelectionMutation({
          variables: {
            input: {
              optionId: op.optionRef,
              productId: op.product.id,
              quantity: op.quantity,
              isDefault: op.isDefault,
              sortOrder: op.sortOrder,
            },
          },
        })
        // Product is unique within an option's selections (the picker filters out already-linked rows), so the new
        // selection is the one whose product.id matches the op
        const opts = res.data?.addBundleSelection.bundleOptions ?? []
        const parentOpt = opts.find((o) => o.id === op.optionRef)
        const fresh = parentOpt?.selections.find((s) => s.product.id === op.product.id)
        if (!fresh) {
          throw new Error('Could not resolve newly-added bundle selection id from response.')
        }
        return fresh.id
      },
      updateSelection: async (op) => {
        const variables: { input: { id: string; quantity?: number; isDefault?: boolean; sortOrder?: number } } = {
          input: { id: op.id },
        }
        if (op.quantity !== undefined) variables.input.quantity = op.quantity
        if (op.isDefault !== undefined) variables.input.isDefault = op.isDefault
        if (op.sortOrder !== undefined) variables.input.sortOrder = op.sortOrder
        await updateSelectionMutation({ variables })
      },
      deleteSelection: async (op) => {
        await deleteSelectionMutation({ variables: { id: op.id } })
      },
    }

    try {
      await commitOps(reduced, product.id, committers)
    } catch (e) {
      // keep only the unapplied tail - a retry resumes instead of re-running committed adds and duplicating selections
      if (e instanceof CommitError) setPendingOps(e.remaining as PendingOp[])
      throw e
    }
    setPendingOps([])
  }, [
    reduced,
    product.id,
    product.bundleOptions,
    addOptionMutation,
    updateOptionMutation,
    upsertOptionContentMutation,
    deleteOptionMutation,
    addSelectionMutation,
    updateSelectionMutation,
    deleteSelectionMutation,
    deleteOptionContentMutation,
  ])

  useProductSaveSection({ id: 'bundle-structure', title: 'Bundle structure', diff, save })

  // Pricing per store, staged like the structure - its own section, saved after the listings so a store listed in the same Save exists by then
  const listings = useMemo(() => product.listings ?? [], [product.listings])
  const [stagedPricing, setStagedPricing] = useState<Record<string, StorePricing>>({})
  const savedPricing = useCallback(
    (l: BundleListing): StorePricing => ({ mode: l.bundlePricing ?? 'SUM', percent: l.bundleDiscountPercent ?? 0 }),
    [],
  )
  const pricingFor = useCallback((l: BundleListing) => stagedPricing[l.channel.id] ?? savedPricing(l), [stagedPricing, savedPricing])
  const pricingChanges = useMemo(
    () =>
      listings.flatMap((l) => {
        const next = stagedPricing[l.channel.id]
        const was = savedPricing(l)
        return next && (next.mode !== was.mode || (next.mode === 'SUM_MINUS_PERCENT' && next.percent !== was.percent)) ? [{ l, was, next }] : []
      }),
    [listings, stagedPricing, savedPricing],
  )
  const [setPricingMutation] = useMutation(SetBundlePricingDocument, { refetchQueries: refetch })
  const savePricing = useCallback(async () => {
    for (const { l, next } of pricingChanges) {
      await setPricingMutation({ variables: { input: { productId: product.id, channelId: l.channel.id, pricing: next.mode, discountPercent: next.percent } } })
      // drop each store as it lands, so a retry after a failure doesn't resend the ones that saved
      setStagedPricing((prev) => {
        const rest = { ...prev }
        delete rest[l.channel.id]
        return rest
      })
    }
  }, [pricingChanges, setPricingMutation, product.id])
  useProductSaveSection({
    id: 'bundle-pricing',
    title: 'Bundle pricing',
    priority: 10,
    diff: pricingChanges.map(({ l, was, next }) => ({ label: `Pricing · ${l.channel.name}`, before: pricingText(was), after: pricingText(next) })),
    save: savePricing,
  })

  const [addOptionOpen, setAddOptionOpen] = useState(false)
  const [editingOption, setEditingOption] = useState<BundleOption | null>(null)
  const [confirmingDeleteOption, setConfirmingDeleteOption] = useState<BundleOption | null>(null)
  const [pickerOption, setPickerOption] = useState<BundleOption | null>(null)
  const [editingSelection, setEditingSelection] = useState<{ option: BundleOption; selection: Selection } | null>(null)
  const [confirmingDeleteSelection, setConfirmingDeleteSelection] = useState<{ option: BundleOption; selection: Selection } | null>(null)

  const sortedOptions = useMemo(
    () => [...staged].sort((a, b) => a.sortOrder - b.sortOrder),
    [staged],
  )

  function onDeleteOption(option: BundleOption) {
    stage({ kind: 'delete-option', id: option.id })
  }
  function onDeleteSelection(selection: Selection) {
    stage({ kind: 'delete-selection', id: selection.id })
  }

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <div className="grid gap-1">
          <h3 className="text-sm font-medium">Bundle structure</h3>
          <p className="text-sm text-muted-foreground">
            Define one or more options. Each option offers a menu of simple-product selections - the
            customer picks per option at add-to-cart. Edits batch into the page-level "Save changes".
          </p>
        </div>
        {canManage && (
          <Button size="sm" onClick={() => setAddOptionOpen(true)}>
            <PlusIcon className="size-4" />
            Add option
          </Button>
        )}
      </div>

      {sortedOptions.length > 0 && (
        <BundleHealth
          options={sortedOptions}
          listings={listings}
          pricingFor={pricingFor}
          onPricingChange={canManage ? (channelId, p) => setStagedPricing((prev) => ({ ...prev, [channelId]: p })) : undefined}
        />
      )}

      {sortedOptions.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
          No options yet. Add one to start defining what the customer can pick.
        </p>
      ) : (
        <div className="grid gap-4">
          {sortedOptions.map((opt) => (
            <OptionCard
              key={opt.id}
              option={opt}
              canManage={canManage}
              onEditOption={() => setEditingOption(opt)}
              onDeleteOption={() => setConfirmingDeleteOption(opt)}
              onAddSelection={() => setPickerOption(opt)}
              onEditSelection={(sel) => setEditingSelection({ option: opt, selection: sel })}
              onDeleteSelection={(sel) => setConfirmingDeleteSelection({ option: opt, selection: sel })}
            />
          ))}
        </div>
      )}

      {canManage && (
        <AddOptionDialog
          locale={product.contentLocale ?? 'en'}
          open={addOptionOpen}
          onOpenChange={setAddOptionOpen}
          existingMaxSort={sortedOptions.reduce((m, o) => Math.max(m, o.sortOrder), -1)}
          onStage={stage}
        />
      )}
      {canManage && editingOption && (
        <EditOptionDialog
          option={editingOption}
          locale={product.contentLocale ?? 'en'}
          open
          onOpenChange={(v) => { if (!v) setEditingOption(null) }}
          onStage={stage}
        />
      )}
      {canManage && pickerOption && (
        <SelectionPickerSheet
          option={pickerOption}
          productId={product.id}
          open
          onOpenChange={(v) => { if (!v) setPickerOption(null) }}
          onStage={stage}
        />
      )}
      {canManage && editingSelection && (
        <EditSelectionDialog
          option={editingSelection.option}
          selection={editingSelection.selection}
          open
          onOpenChange={(v) => { if (!v) setEditingSelection(null) }}
          onStage={stage}
        />
      )}

      <ConfirmDialog
        open={!!confirmingDeleteOption}
        onOpenChange={(v) => { if (!v) setConfirmingDeleteOption(null) }}
        title={<>Delete option "{confirmingDeleteOption?.title}"?</>}
        description="Stages the option (and every selection under it) for removal. The linked simple products themselves are not deleted. The change takes effect when you confirm the page-level save."
        confirmLabel="Stage delete"
        destructive
        onConfirm={() => { if (confirmingDeleteOption) onDeleteOption(confirmingDeleteOption) }}
      />

      <ConfirmDialog
        open={!!confirmingDeleteSelection}
        onOpenChange={(v) => { if (!v) setConfirmingDeleteSelection(null) }}
        title={<>Remove {confirmingDeleteSelection?.selection.product.name ?? confirmingDeleteSelection?.selection.product.sku ?? ''} from this option?</>}
        description="Stages the selection for removal. The product itself isn't deleted - only the link is removed when you confirm the page-level save."
        confirmLabel="Stage remove"
        destructive
        onConfirm={() => { if (confirmingDeleteSelection) onDeleteSelection(confirmingDeleteSelection.selection) }}
      />
    </div>
  )
}

function IssueLine({ issue }: { issue: HealthIssue }) {
  return (
    <li className={`flex items-start gap-1.5 text-xs ${issue.level === 'destructive' ? 'text-destructive' : 'text-warning'}`}>
      <AlertTriangleIcon className="mt-0.5 size-3 shrink-0" />
      {issue.text}
    </li>
  )
}

const PRICING_LABEL: Record<BundlePricing, string> = {
  SUM: 'Sum of the picks',
  FIXED: 'Fixed price',
  SUM_MINUS_PERCENT: '% off the picks',
}

function pricingText(p: StorePricing) {
  return p.mode === 'SUM_MINUS_PERCENT' ? `${p.percent}% off the picks` : PRICING_LABEL[p.mode]
}

function BundlePricingControl({ store, value, onChange }: { store: string; value: StorePricing; onChange: (p: StorePricing) => void }) {
  return (
    <div className="flex items-center gap-2">
      <Select value={value.mode} onValueChange={(v) => onChange({ mode: v as BundlePricing, percent: v === 'SUM_MINUS_PERCENT' ? value.percent || 10 : 0 })}>
        <SelectTrigger size="sm" className="h-7 w-40 text-xs" aria-label={`${store} bundle pricing`}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(Object.keys(PRICING_LABEL) as BundlePricing[]).map((m) => (
            <SelectItem key={m} value={m}>{PRICING_LABEL[m]}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      {value.mode === 'SUM_MINUS_PERCENT' && (
        <Input
          type="number"
          inputMode="numeric"
          min={0}
          max={100}
          className="h-7 w-16 text-xs"
          aria-label={`${store} discount percent`}
          value={value.percent}
          onChange={(e) => onChange({ ...value, percent: Math.min(100, Math.max(0, Math.round(Number(e.target.value) || 0))) })}
        />
      )}
    </div>
  )
}

// What customers can actually buy on each store the bundle is listed on, from the staged structure so edits show before the save
function BundleHealth({
  options,
  listings,
  pricingFor,
  onPricingChange,
}: {
  options: BundleOption[]
  listings: BundleListing[]
  pricingFor: (l: BundleListing) => StorePricing
  onPricingChange?: (channelId: string, p: StorePricing) => void
}) {
  const general = useMemo(() => structureIssues(options), [options])
  const stores = useMemo(() => storeHealth(listings, options, pricingFor), [listings, options, pricingFor])
  const unsaved = options.some((o) => o.selections.some((s) => !s.product.listings))
  return (
    <div className="divide-y rounded-md border text-sm" aria-label="Bundle health">
      {general.length > 0 && (
        <ul className="grid gap-1 px-3 py-2">{general.map((i) => <IssueLine key={i.text} issue={i} />)}</ul>
      )}
      {stores.length === 0 && (
        <p className="px-3 py-2 text-xs text-muted-foreground">Not listed on any store yet - list it to see what customers pay.</p>
      )}
      {stores.map((st, i) => (
        <div key={st.channelId} className="grid gap-1 px-3 py-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="font-medium">{st.name}</span>
            <div className="flex flex-wrap items-center gap-3">
              {st.price && (
                <span className="text-xs text-muted-foreground">
                  Customers pay {formatCents(st.price.min, st.currencyCode)}
                  {st.price.max !== st.price.min && <>–{formatCents(st.price.max, st.currencyCode)}</>}
                  , default {formatCents(st.price.default, st.currencyCode)}
                  {st.sum && (
                    <>
                      {' '}· picks add up to {formatCents(st.sum.min, st.currencyCode)}
                      {st.sum.max !== st.sum.min && <>–{formatCents(st.sum.max, st.currencyCode)}</>}
                    </>
                  )}
                </span>
              )}
              {onPricingChange ? (
                <BundlePricingControl store={st.name} value={pricingFor(listings[i])} onChange={(p) => onPricingChange(st.channelId, p)} />
              ) : (
                <span className="text-xs text-muted-foreground">{pricingText(pricingFor(listings[i]))}</span>
              )}
            </div>
          </div>
          {st.issues.length === 0 ? (
            <p className="flex items-center gap-1.5 text-xs text-success">
              <CheckCircle2Icon className="size-3" />
              Every selection can be bought here.
            </p>
          ) : (
            <ul className="grid gap-1">{st.issues.map((i) => <IssueLine key={i.text} issue={i} />)}</ul>
          )}
        </div>
      ))}
      {unsaved && <p className="px-3 py-2 text-xs text-muted-foreground">New selections are checked after you save.</p>}
    </div>
  )
}

function OptionCard({
  option,
  canManage,
  onEditOption,
  onDeleteOption,
  onAddSelection,
  onEditSelection,
  onDeleteSelection,
}: {
  option: BundleOption
  canManage: boolean
  onEditOption: () => void
  onDeleteOption: () => void
  onAddSelection: () => void
  onEditSelection: (sel: Selection) => void
  onDeleteSelection: (sel: Selection) => void
}) {
  const pending = isTemp(option.id)
  const sortedSelections = useMemo(
    () => [...option.selections].sort((a, b) => a.sortOrder - b.sortOrder),
    [option.selections],
  )
  return (
    <Card className="grid gap-3 p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <PackagePlusIcon className="size-4 text-muted-foreground" />
            <span className="font-medium">{option.title || <span className="text-muted-foreground">Untitled option</span>}</span>
            <Badge variant="outline" className="font-normal">{INPUT_TYPE_LABEL[option.inputType]}</Badge>
            {!isSingleSelect(option.inputType) && selectionLimitText(option.minSelections, option.maxSelections) && (
              <Badge variant="outline" className="font-normal">{selectionLimitText(option.minSelections, option.maxSelections)}</Badge>
            )}
            {option.isRequired ? (
              <Badge variant="secondary" className="font-normal">Required</Badge>
            ) : (
              <Badge variant="outline" className="font-normal text-muted-foreground">Optional</Badge>
            )}
            {pending && (
              <Badge variant="outline" className="font-normal text-warning">Unsaved</Badge>
            )}
          </div>
          {option.content.length > 1 && (
            <p className="text-xs text-muted-foreground">
              {option.content.length} translation{option.content.length === 1 ? '' : 's'} on file.
            </p>
          )}
        </div>
        {canManage && (
          <div className="flex shrink-0 gap-1">
            <Button variant="ghost" size="icon" className="size-7" onClick={onEditOption} aria-label="Edit option">
              <PencilIcon className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon" className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={onDeleteOption} aria-label="Delete option">
              <Trash2Icon className="size-3.5" />
            </Button>
          </div>
        )}
      </div>

      {sortedSelections.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-4 text-center text-xs text-muted-foreground">
          No selections yet. Add one to fill this option's menu.
        </p>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Product</TableHead>
                <TableHead className="w-20">SKU</TableHead>
                <TableHead className="w-20">Qty</TableHead>
                <TableHead className="w-24">Price</TableHead>
                <TableHead className="w-20">Default</TableHead>
                {canManage && <TableHead className="w-20" />}
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedSelections.map((sel) => {
                const selPending = isTemp(sel.id)
                return (
                  <TableRow key={sel.id} className={selPending ? 'bg-warning-soft' : ''}>
                    <TableCell className="font-medium">
                      {sel.product.name ?? sel.product.sku ?? ''}
                      {selPending && <span className="ml-2 text-xs text-warning">· unsaved</span>}
                    </TableCell>
                    <TableCell>
                      {sel.product.sku ? (
                        <code className="font-mono text-xs">{sel.product.sku}</code>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{sel.quantity}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {formatCents(sel.product.defaultPrice, sel.product.defaultCurrencyCode)}
                    </TableCell>
                    <TableCell>
                      {sel.isDefault ? (
                        <CheckCircle2Icon className="size-4 text-primary" />
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    {canManage && (
                      <TableCell>
                        <div className="flex gap-1">
                          <Button variant="ghost" size="icon" className="size-7" onClick={() => onEditSelection(sel)} aria-label="Edit selection">
                            <PencilIcon className="size-3.5" />
                          </Button>
                          <Button variant="ghost" size="icon" className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => onDeleteSelection(sel)} aria-label="Remove selection">
                            <Trash2Icon className="size-3.5" />
                          </Button>
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

      {canManage && (
        <div className="flex justify-end">
          <Button variant="outline" size="sm" onClick={onAddSelection}>
            <PlusIcon className="size-4" />
            Add selection
          </Button>
        </div>
      )}
    </Card>
  )
}

// ──────────────────────── Add option ────────────────────────

function AddOptionDialog({
  locale,
  open,
  onOpenChange,
  existingMaxSort,
  onStage,
}: {
  locale: string
  open: boolean
  onOpenChange: (v: boolean) => void
  existingMaxSort: number
  onStage: (op: PendingOp) => void
}) {
  const [title, setTitle] = useState('')
  const [inputType, setInputType] = useState<BundleInputType>('RADIO')
  const [isRequired, setIsRequired] = useState(true)
  const [limits, setLimits] = useState<Limits>(NO_LIMITS)
  const bounds = effectiveLimits(inputType, limits)
  const limitsErr = limitsError(bounds)

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (limitsErr) return
    onStage({
      kind: 'add-option',
      tempId: newTempId(),
      title: title.trim(),
      inputType,
      isRequired,
      sortOrder: existingMaxSort + 1,
      minSelections: bounds.min,
      maxSelections: bounds.max,
      locale,
    })
    setTitle('')
    setInputType('RADIO')
    setIsRequired(true)
    setLimits(NO_LIMITS)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add bundle option</DialogTitle>
          <DialogDescription>
            Title and shape for one "Pick a thing" slot on this bundle. Stages until the page-level save.
          </DialogDescription>
        </DialogHeader>

        <form id="add-bundle-option" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="opt-title">Title</Label>
            <Input
              id="opt-title"
              required
              placeholder="Pick a shirt"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              First-translation title in <code className="font-mono">{locale}</code>. More locales can be added later.
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="opt-input">Customer picks</Label>
            <Select value={inputType} onValueChange={(v) => setInputType(v as BundleInputType)}>
              <SelectTrigger id="opt-input">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {INPUT_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {INPUT_TYPE_LABEL[t]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-md border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="opt-required" className="text-sm">Required</Label>
              <p className="text-xs text-muted-foreground">
                When on, the customer must pick a selection at add-to-cart.
              </p>
            </div>
            <Switch id="opt-required" checked={isRequired} onCheckedChange={setIsRequired} />
          </div>

          {!isSingleSelect(inputType) && <SelectionLimitsFields idPrefix="opt" value={limits} onChange={setLimits} error={limitsErr} />}
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button type="submit" form="add-bundle-option" disabled={!title.trim() || !!limitsErr}>
            Stage add
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ──────────────────────── Edit option ────────────────────────

function EditOptionDialog({
  option,
  locale,
  open,
  onOpenChange,
  onStage,
}: {
  option: BundleOption
  // The page's content locale - the title is read and written in it, so editing in a translated view never overwrites another locale
  locale: string
  open: boolean
  onOpenChange: (v: boolean) => void
  onStage: (op: PendingOp) => void
}) {
  const [inputType, setInputType] = useState<BundleInputType>(option.inputType)
  const [isRequired, setIsRequired] = useState(option.isRequired)
  const [sortOrder, setSortOrder] = useState(option.sortOrder)
  const [limits, setLimits] = useState<Limits>({ min: option.minSelections, max: option.maxSelections })
  const bounds = effectiveLimits(inputType, limits)
  const limitsErr = limitsError(bounds)
  const titleLocale = locale
  // No translation yet shows the fallback title - left as is, it stages nothing, typed over, it adds the locale
  const initialTitle = option.content.find((c) => c.locale === locale)?.title ?? option.title
  const [title, setTitle] = useState(initialTitle)

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const op = {
      kind: 'update-option' as const,
      id: option.id,
      ...(title.trim() !== initialTitle ? { title: title.trim(), titleLocale } : {}),
      ...(inputType !== option.inputType ? { inputType } : {}),
      ...(isRequired !== option.isRequired ? { isRequired } : {}),
      ...(sortOrder !== option.sortOrder ? { sortOrder } : {}),
      ...(bounds.min !== option.minSelections ? { minSelections: bounds.min } : {}),
      ...(bounds.max !== option.maxSelections ? { maxSelections: bounds.max } : {}),
    }
    if (limitsErr) return
    // Only stage when there's something to change
    if (Object.keys(op).length > 2) {
      onStage(op)
    }
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit option</DialogTitle>
          <DialogDescription>
            Title in the page's locale, how the customer picks, required flag, and sort order. Stages until the page-level save.
          </DialogDescription>
        </DialogHeader>

        <form id="edit-bundle-option" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="opt-title-edit">Title ({titleLocale})</Label>
            <Input
              id="opt-title-edit"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              required
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="opt-input-edit">Customer picks</Label>
              <Select value={inputType} onValueChange={(v) => setInputType(v as BundleInputType)}>
                <SelectTrigger className="w-full" id="opt-input-edit">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {INPUT_TYPES.map((t) => (
                    <SelectItem key={t} value={t}>{INPUT_TYPE_LABEL[t]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="opt-sort-edit">Sort order</Label>
              <Input
                id="opt-sort-edit"
                type="number"
                inputMode="numeric"
                value={sortOrder}
                onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
              />
            </div>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-md border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="opt-required-edit" className="text-sm">Required</Label>
              <p className="text-xs text-muted-foreground">Customer must pick a selection at add-to-cart.</p>
            </div>
            <Switch id="opt-required-edit" checked={isRequired} onCheckedChange={setIsRequired} />
          </div>

          {!isSingleSelect(inputType) && (
            <SelectionLimitsFields idPrefix="opt-edit" value={limits} onChange={setLimits} error={limitsErr} selectionCount={option.selections.length} />
          )}
        </form>

        {!isTemp(option.id) && (
          <TranslationLocales
            locale={titleLocale}
            locales={option.content.map((c) => c.locale)}
            entityLabel="bundle option"
            canManage
            defaultLocaleLocked
            staged
            onRemove={async () => {
              onStage({ kind: 'delete-content', id: option.id, locale: titleLocale })
              onOpenChange(false)
            }}
          />
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button type="submit" form="edit-bundle-option" disabled={!!limitsErr}>Stage edit</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ──────────────────────── Edit selection ────────────────────────

function EditSelectionDialog({
  option,
  selection,
  open,
  onOpenChange,
  onStage,
}: {
  option: BundleOption
  selection: Selection
  open: boolean
  onOpenChange: (v: boolean) => void
  onStage: (op: PendingOp) => void
}) {
  const [quantity, setQuantity] = useState(selection.quantity)
  const [isDefault, setIsDefault] = useState(selection.isDefault)
  const [sortOrder, setSortOrder] = useState(selection.sortOrder)
  const [err, setErr] = useState<string | null>(null)

  const otherDefaultExists = useMemo(
    () => option.selections.some((s) => s.id !== selection.id && s.isDefault),
    [option.selections, selection.id],
  )
  const blockedByCardinality = isSingleSelect(option.inputType) && isDefault && !selection.isDefault && otherDefaultExists

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setErr(null)
    if (blockedByCardinality) {
      setErr('A pick-one option accepts at most one default selection - clear the other default first.')
      return
    }
    const op = {
      kind: 'update-selection' as const,
      id: selection.id,
      ...(quantity !== selection.quantity ? { quantity } : {}),
      ...(isDefault !== selection.isDefault ? { isDefault } : {}),
      ...(sortOrder !== selection.sortOrder ? { sortOrder } : {}),
    }
    if (Object.keys(op).length > 2) onStage(op)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit selection</DialogTitle>
          <DialogDescription>
            {selection.product.name ?? selection.product.sku ?? ''}
          </DialogDescription>
        </DialogHeader>

        <form id="edit-bundle-selection" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="sel-quantity-edit">Quantity</Label>
              <Input
                id="sel-quantity-edit"
                type="number"
                min={1}
                value={quantity}
                onChange={(e) => setQuantity(Math.max(1, Number(e.target.value) || 1))}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="sel-sort-edit">Sort order</Label>
              <Input
                id="sel-sort-edit"
                type="number"
                inputMode="numeric"
                value={sortOrder}
                onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
              />
            </div>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-md border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="sel-default-edit" className="text-sm">Default</Label>
              <p className="text-xs text-muted-foreground">Pre-checked when the customer opens this bundle.</p>
            </div>
            <Switch id="sel-default-edit" checked={isDefault} onCheckedChange={setIsDefault} />
          </div>

          {err && (
            <Alert variant="destructive">
              <AlertTitle>Can't stage</AlertTitle>
              <AlertDescription>{err}</AlertDescription>
            </Alert>
          )}
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button type="submit" form="edit-bundle-selection">Stage edit</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ──────────────────────── Selection picker ────────────────────────

function SelectionPickerSheet({
  option,
  productId,
  open,
  onOpenChange,
  onStage,
}: {
  option: BundleOption
  productId: string
  open: boolean
  onOpenChange: (v: boolean) => void
  onStage: (op: PendingOp) => void
}) {
  const takenIds = useMemo(() => new Set(option.selections.map((s) => s.product.id)), [option.selections])
  const selfId = useMemo(() => new Set([productId]), [productId])
  const extra = useMemo<PickExtra<{ quantity: number; isDefault: boolean }>>(
    () => ({ initial: () => ({ quantity: 1, isDefault: false }), columns: [quantityColumn(), defaultColumn()] }),
    [],
  )

  return (
    <ProductPickerDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Add selections to "${option.title}"`}
      description="Pick one or more simple products. Per row, set how many of that product the bundle includes and whether it's the default. Stages until the page-level save."
      kinds={['SIMPLE']}
      takenIds={takenIds}
      takenLabel="In this option"
      excludeIds={selfId}
      showPrice
      extra={extra}
      confirmLabel={(n) => `Stage ${n || ''} selection${n === 1 ? '' : 's'}`.replace('  ', ' ')}
      validate={(picks) => {
        // Single-select default sanity check before staging any ops
        if (!isSingleSelect(option.inputType)) return null
        const existingDefaults = option.selections.filter((s) => s.isDefault).length
        const newDefaults = picks.filter((p) => p.extra.isDefault).length
        return existingDefaults + newDefaults > 1
          ? `${INPUT_TYPE_LABEL[option.inputType]} options accept at most one default.`
          : null
      }}
      onConfirm={(picks) => {
        let nextSort = option.selections.length
        for (const { product, extra: params } of picks) {
          onStage({
            kind: 'add-selection',
            tempId: newTempId(),
            optionRef: option.id,
            product: toSelectionProduct(product),
            quantity: params.quantity,
            isDefault: params.isDefault,
            sortOrder: nextSort,
          })
          nextSort++
        }
      }}
    />
  )
}

function toSelectionProduct(p: PickerProduct): SelectionProduct {
  return { id: p.id, sku: p.sku, name: p.name, kind: p.kind, defaultPrice: p.defaultPrice, defaultCurrencyCode: p.defaultCurrencyCode }
}
