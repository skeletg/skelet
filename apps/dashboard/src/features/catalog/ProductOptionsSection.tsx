import { Suspense, lazy, useCallback, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, CopyIcon, EllipsisIcon, EyeIcon, LinkIcon, PencilIcon, PlusIcon, Trash2Icon, Undo2Icon } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ConfiguratorLayout, ProductOptionInputType, ProductOptionPriceType } from '@/graphql/schema.types'
import {
  AddProductOptionDocument,
  AddProductOptionValueDocument,
  DeleteProductOptionDocument,
  DeleteProductOptionValueDocument,
  DetachOptionTemplateDocument,
  DuplicateProductOptionDocument,
  OptionTemplateDocument,
  OptionTemplatesDocument,
  ProductDocument,
  SetOptionTemplateRulesDocument,
  SetProductOptionChannelPriceDocument,
  SetProductConfiguratorLayoutDocument,
  SetProductOptionRulesDocument,
  SetProductOptionTemplatesDocument,
  SetProductOptionValueChannelPriceDocument,
  UpdateProductOptionDocument,
  UpdateProductOptionValueDocument,
  UpsertProductOptionContentDocument,
  UpsertProductOptionValueContentDocument,
  type ProductOptionRuleFieldsFragment,
} from './Catalog.gql'
import {
  applyOps,
  CommitError,
  commitOps,
  isTemp,
  opsToDiff,
  optionContentFor,
  reduceOps,
  valueContentFor,
  type Committers,
  type PendingOp,
  type ProductOption,
  type VisibilityGroup,
} from './productOptionsStaging'
import { useProductSaveSection } from './productSaveRegistry'
import { useConfigScope } from '@/lib/config-scope'
import type { SurchargeChannel } from './channelSurcharges'
import { OptionDialog } from './product-options/OptionDialog'
import { conditionSummary, dependents, priceLabel, type DraftContext } from './product-options/option-draft'
import { INPUT_TYPES, isChoice } from './product-options/option-types'
import { RuleDialog } from './product-options/RuleDialog'
import { RulesList } from './product-options/RulesList'
import { AddTemplateMenu, TemplateGroup } from './product-options/TemplateGroup'
import { TemplatesDialog } from './product-options/TemplatesDialog'
import { templatePath } from './product-options/template-path'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { usePermissions } from '@/shared/auth/usePermissions'
import { resolveRuleIds, ruleLabel, ruleToInput, rulesDiff, rulesFromQuery, rulesUsing, type OptionRule } from './product-options/rules'

// The map pulls in the graph library - loaded only when someone opens it
const OptionsMap = lazy(() => import('./product-options/OptionsMap'))

export type ConfiguratorTemplateRef = { id: string; name: string; optionCount: number; ruleCount: number }
// Whose options these are - a product's list carries the options and rules of its templates too, read-only here:
// first the ones its product type hands it, then the ones it picked
export type ConfiguratorOwner =
  | {
      kind: 'product'
      id: string
      templates: readonly ConfiguratorTemplateRef[]
      typeTemplates: readonly ConfiguratorTemplateRef[]
      productType: { id: string; name: string } | null
      layout: ConfiguratorLayout
    }
  | { kind: 'template'; id: string }
export type ConfiguratorSubject = {
  owner: ConfiguratorOwner
  contentLocale?: string | null
  currency: string
  channels: SurchargeChannel[]
  productOptions: ProductOption[]
  optionRules?: readonly ProductOptionRuleFieldsFragment[]
}

type WithOptions = {
  id: string
  contentLocale?: string | null
  defaultCurrencyCode?: string
  listings?: { channel: SurchargeChannel }[]
  productOptions: ProductOption[]
  optionRules?: readonly ProductOptionRuleFieldsFragment[]
  optionTemplates?: readonly ConfiguratorTemplateRef[]
  typeOptionTemplates?: readonly ConfiguratorTemplateRef[]
  productType?: { id: string; name: string } | null
  configuratorLayout?: ConfiguratorLayout
}

const MAX_CHIPS = 8
const LAYOUT_LABEL: Record<ConfiguratorLayout, string> = { LIST: 'All at once', STEPS: 'Step by step' }

export function ProductOptionsSection({ product, canManage }: { product: WithOptions; canManage: boolean }) {
  const subject = useMemo<ConfiguratorSubject>(
    () => ({
      owner: {
        kind: 'product',
        id: product.id,
        templates: product.optionTemplates ?? [],
        typeTemplates: product.typeOptionTemplates ?? [],
        productType: product.productType ?? null,
        layout: product.configuratorLayout ?? 'LIST',
      },
      contentLocale: product.contentLocale,
      currency: product.defaultCurrencyCode ?? '',
      channels: (product.listings ?? []).map((l) => l.channel),
      productOptions: product.productOptions,
      optionRules: product.optionRules,
    }),
    [product],
  )
  return <ConfiguratorSection subject={subject} canManage={canManage} />
}

export function ConfiguratorSection({ subject, canManage }: { subject: ConfiguratorSubject; canManage: boolean }) {
  const { owner } = subject
  const navigate = useNavigate()
  // Only the owner's own options stage and save here - a product's template options are edited on the template
  // On a template every option is its own, even though each names the template
  const inheritedFrom = useCallback((templateId?: string | null) => owner.kind === 'product' && !!templateId, [owner.kind])
  const own = useMemo(() => subject.productOptions.filter((o) => !inheritedFrom(o.templateId)), [subject.productOptions, inheritedFrom])
  const inherited = useMemo(
    () => subject.productOptions.filter((o) => inheritedFrom(o.templateId)).sort((a, b) => a.sortOrder - b.sortOrder),
    [subject.productOptions, inheritedFrom],
  )
  // Kept as staged, not pre-collapsed, so undoing a delete brings back the edits made before it
  const [pendingOps, setPendingOps] = useState<PendingOp[]>([])
  const stage = useCallback((ops: PendingOp[]) => {
    if (ops.length > 0) setPendingOps((prev) => [...prev, ...ops])
  }, [])

  const reduced = useMemo(() => reduceOps(pendingOps), [pendingOps])
  const staged = useMemo(() => applyOps(own, reduced), [own, reduced])
  const sorted = useMemo(() => [...staged].sort((a, b) => a.sortOrder - b.sortOrder), [staged])
  // What conditions, rules and the map may read - the templates' options always come first
  const everything = useMemo(() => [...inherited, ...sorted], [inherited, sorted])

  // Rules stage as one list against the saved one, and save in a single write
  const savedRules = useMemo(
    () => rulesFromQuery((subject.optionRules ?? []).filter((r) => !inheritedFrom(r.templateId))),
    [subject.optionRules, inheritedFrom],
  )
  const inheritedRules = useMemo(
    () =>
      (subject.optionRules ?? []).flatMap((r) =>
        inheritedFrom(r.templateId) ? rulesFromQuery([r]).map((rule) => ({ rule, template: r.templateName ?? '' })) : [],
      ),
    [subject.optionRules, inheritedFrom],
  )
  const [rulesDraft, setRulesDraft] = useState<OptionRule[] | null>(null)
  const rules = rulesDraft ?? savedRules
  const rulesChanged = rulesDraft !== null && JSON.stringify(rulesDraft.map(ruleToInput)) !== JSON.stringify(savedRules.map(ruleToInput))
  const ctx = useMemo<DraftContext>(() => ({ channels: subject.channels, baseCurrency: subject.currency }), [subject.channels, subject.currency])
  const diff = useMemo(() => {
    const byId = new Map(ctx.channels.map((c) => [c.id, c]))
    const label = (id: string) => {
      const c = byId.get(id)
      return c ? `${c.name} (${c.currencyCode})` : 'another channel'
    }
    return opsToDiff(own, reduced, label)
  }, [own, reduced, ctx.channels])

  // New options/values must be created in the configured default locale (the backend enforces this); translations are
  // added afterwards by switching the language
  const defaultLocale = subject.contentLocale ?? 'en'
  const { effectiveLocale, availableLocales } = useConfigScope()
  const [editLocale, setEditLocale] = useState(effectiveLocale || defaultLocale)
  const localeOptions = useMemo(() => {
    const seen = new Set<string>()
    const out: string[] = []
    const push = (l: string) => {
      if (l && !seen.has(l)) { seen.add(l); out.push(l) }
    }
    push(defaultLocale)
    for (const l of availableLocales) push(l)
    for (const o of subject.productOptions) for (const c of o.content ?? []) push(c.locale)
    push(editLocale)
    return out
  }, [availableLocales, subject.productOptions, defaultLocale, editLocale])

  const refetch = useMemo(
    () =>
      owner.kind === 'product'
        ? [{ query: ProductDocument, variables: { id: owner.id } }]
        : [{ query: OptionTemplateDocument, variables: { id: owner.id } }, { query: OptionTemplatesDocument }],
    [owner.kind, owner.id],
  )
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
  const [setRulesMutation] = useMutation(SetProductOptionRulesDocument, { refetchQueries: refetch })
  const [setTemplateRulesMutation] = useMutation(SetOptionTemplateRulesDocument, { refetchQueries: refetch })
  const [setLayoutMutation] = useMutation(SetProductConfiguratorLayoutDocument, { refetchQueries: refetch })
  // How the product page asks them - a product setting, staged with the rest of the section
  const savedLayout: ConfiguratorLayout = owner.kind === 'product' ? owner.layout : 'LIST'
  const [layoutDraft, setLayoutDraft] = useState<ConfiguratorLayout | null>(null)
  const layout = layoutDraft ?? savedLayout
  const layoutChanged = layout !== savedLayout

  const save = useCallback(async () => {
    const ops: PendingOp[] = rulesChanged && rulesDraft ? [...reduced, { kind: 'set-rules', rules: rulesDraft.map(ruleToInput) }] : reduced
    if (ops.length === 0 && !layoutChanged) return
    const knownOptionIds = new Set(subject.productOptions.map((o) => o.id))
    const knownValueIds = new Set<string>()
    for (const o of subject.productOptions) for (const v of o.values) knownValueIds.add(v.id)

    const committers: Committers = {
      addOption: async (op) => {
        const res = await addOptionMutation({
          variables: {
            input: {
              ...(owner.kind === 'product' ? { productId: op.productId } : { templateId: op.productId }),
              inputType: op.inputType,
              isRequired: op.isRequired,
              sortOrder: op.sortOrder,
              priceValue: op.priceValue ?? undefined,
              priceType: op.priceType ?? undefined,
              maxChars: op.maxChars ?? undefined,
              maxFileBytes: op.maxFileBytes ?? undefined,
              fileExtensions: op.fileExtensions ?? undefined,
              visibleIf: op.visibleIf ?? undefined,
              imageAssetId: op.image?.assetId ?? undefined,
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
            imageAssetId?: string
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
        // An empty id is the schema's "remove the picture"
        if (op.image !== undefined) variables.input.imageAssetId = op.image?.assetId ?? ''
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
              imageAssetId: op.image?.assetId ?? undefined,
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
            imageAssetId?: string
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
        if (op.image !== undefined) variables.input.imageAssetId = op.image?.assetId ?? ''
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
      setRules: async (rules) => {
        if (owner.kind === 'product') await setRulesMutation({ variables: { productId: owner.id, rules } })
        else await setTemplateRulesMutation({ variables: { templateId: owner.id, rules } })
      },
    }

    try {
      if (ops.length > 0) await commitOps(ops, owner.id, committers)
      if (layoutChanged) await setLayoutMutation({ variables: { productId: owner.id, layout } })
    } catch (e) {
      // keep only the unapplied tail - a retry resumes instead of
      // re-running committed adds and duplicating options/values
      if (e instanceof CommitError) {
        setPendingOps(e.remaining.filter((op) => op.kind !== 'set-rules'))
        const idMap = e.idMap
        setRulesDraft((draft) =>
          e.remaining.some((op) => op.kind === 'set-rules') && draft ? resolveRuleIds(draft, (id) => idMap.get(id) ?? id) : null,
        )
      }
      throw e
    }
    setPendingOps([])
    setRulesDraft(null)
    setLayoutDraft(null)
  }, [
    layout,
    layoutChanged,
    setLayoutMutation,
    reduced,
    rulesChanged,
    rulesDraft,
    setRulesMutation,
    setTemplateRulesMutation,
    owner.kind,
    owner.id,
    subject.productOptions,
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

  const allDiff = useMemo(
    () => [
      ...diff,
      ...(rulesChanged && rulesDraft ? rulesDiff(savedRules, rulesDraft, everything, defaultLocale, ctx.baseCurrency) : []),
      ...(layoutChanged ? [{ kind: 'change' as const, label: 'Shown as', before: LAYOUT_LABEL[savedLayout], after: LAYOUT_LABEL[layout] }] : []),
    ],
    [diff, rulesChanged, rulesDraft, savedRules, everything, defaultLocale, ctx.baseCurrency, layoutChanged, savedLayout, layout],
  )
  useProductSaveSection({
    id: owner.kind === 'product' ? 'product-options' : 'template-options',
    title: owner.kind === 'product' ? 'Configurator' : 'Options and rules',
    diff: allDiff,
    save,
  })

  const [editor, setEditor] = useState<{ optionId: string | null } | null>(null)
  const [ruleEditor, setRuleEditor] = useState<{ ruleId: string | null } | null>(null)
  const [view, setView] = useState<'list' | 'map'>('list')
  const editingRule = ruleEditor?.ruleId ? rules.find((r) => r.id === ruleEditor.ruleId) ?? null : null
  // What stops an option or one of its choices from going - a condition or a rule still reading it
  const usersOf = (ref: { optionId?: string; valueId?: string }) => [
    ...dependents(sorted, ref).map((o) => `the condition on ${optionContentFor(o, editLocale).title || 'Untitled option'}`),
    ...rulesUsing(rules, ref).map((r) => {
      const name = ruleLabel(r, editLocale, defaultLocale)
      return name ? `the rule "${name}"` : `rule ${rules.indexOf(r) + 1}`
    }),
  ]
  const editing = editor?.optionId ? sorted.find((o) => o.id === editor.optionId) ?? null : null

  // Deleted rows stay in place, struck through, until the page is saved - so a slip can be undone
  const deletedIds = useMemo(() => new Set(reduced.flatMap((op) => (op.kind === 'delete-option' ? [op.id] : []))), [reduced])
  const rows = useMemo(
    () =>
      [...sorted, ...own.filter((o) => deletedIds.has(o.id))].sort((a, b) => a.sortOrder - b.sortOrder),
    [sorted, own, deletedIds],
  )
  const touched = useMemo(() => {
    const valueOwner = new Map(staged.flatMap((o) => o.values.map((v) => [v.id, o.id] as const)))
    const out = new Set<string>()
    for (const op of reduced) {
      if (op.kind === 'update-option') out.add(op.id)
      else if (op.kind === 'add-value') out.add(op.optionRef)
      else if (op.kind === 'update-value' || op.kind === 'delete-value') {
        const optionId = valueOwner.get(op.id) ?? own.find((o) => o.values.some((v) => v.id === op.id))?.id
        if (optionId) out.add(optionId)
      }
    }
    return out
  }, [reduced, staged, own])

  // Every option moves into fresh numbers above all current ones, which the save writes last-first - see phaseWrites
  const move = (from: number, to: number) => {
    const next = [...sorted]
    const [item] = next.splice(from, 1)
    next.splice(to, 0, item)
    const top = Math.max(-1, ...sorted.map((o) => o.sortOrder), ...own.map((o) => o.sortOrder)) + 1
    stage(next.map((o, i) => ({ kind: 'update-option' as const, id: o.id, sortOrder: top + i })))
  }
  // A swap that would put a condition above the option it reads is refused here - the backend would refuse it at save
  const blockedSwap = (above: ProductOption, below: ProductOption) => dependents([below], { optionId: above.id }).length > 0

  // Templates on a product change right away - they restructure what the product carries rather than edit it
  const templates = owner.kind === 'product' ? owner.templates : []
  const typeTemplates = owner.kind === 'product' ? owner.typeTemplates : []
  const typeIds = useMemo(() => new Set((owner.kind === 'product' ? owner.typeTemplates : []).map((t) => t.id)), [owner])
  const { allowedChannelScopes } = usePermissions()
  const [managing, setManaging] = useState(false)
  const [setTemplatesMutation] = useMutation(SetProductOptionTemplatesDocument, { refetchQueries: [{ query: OptionTemplatesDocument }] })
  const [detachMutation] = useMutation(DetachOptionTemplateDocument, { refetchQueries: [{ query: OptionTemplatesDocument }] })
  const [templateError, setTemplateError] = useState<string | null>(null)
  const [confirmTemplate, setConfirmTemplate] = useState<{ kind: 'remove' | 'detach'; id: string; name: string } | null>(null)
  const setTemplates = async (ids: string[]) => {
    setTemplateError(null)
    try {
      await setTemplatesMutation({ variables: { productId: owner.id, templateIds: ids } })
    } catch (e) {
      setTemplateError(e instanceof Error ? e.message : 'Could not change the templates.')
    }
  }
  const dirty = reduced.length > 0 || rulesChanged
  const leaningOn = (templateId: string) => {
    // Its type keeps handing it over, so taking the pick off loses nothing
    if (typeIds.has(templateId)) return []
    const theirs = inherited.filter((o) => o.templateId === templateId)
    return [
      ...new Set(
        theirs.flatMap((o) => [
          ...usersOf({ optionId: o.id }),
          ...o.values.flatMap((v) => usersOf({ valueId: v.id })),
        ]),
      ),
    ]
  }

  const nextSortOrder = sorted.reduce((m, o) => Math.max(m, o.sortOrder), -1) + 1
  const multilingual = localeOptions.length > 1
  const addButton = canManage ? (
    <Button size="sm" onClick={() => setEditor({ optionId: null })}>
      <PlusIcon className="size-4" />
      Add option
    </Button>
  ) : null
  const templateButton =
    canManage && owner.kind === 'product' ? (
      <AddTemplateMenu
        usedIds={new Set([...templates, ...typeTemplates].map((t) => t.id))}
        onAdd={(id) => void setTemplates([...templates.map((t) => t.id), id])}
        onManage={() => setManaging(true)}
      />
    ) : null
  const openOption = (id: string) => {
    const fromTemplate = inherited.find((o) => o.id === id)
    if (fromTemplate?.templateId) navigate(templatePath(fromTemplate.templateId))
    else if (canManage) setEditor({ optionId: id })
  }
  const anything = rows.length > 0 || templates.length > 0 || typeTemplates.length > 0
  const inheritedRow = (opt: ProductOption) => (
    <OptionRow
      key={opt.id}
      option={opt}
      all={everything}
      locale={editLocale}
      currency={opt.currencyCode ?? ctx.baseCurrency}
      canManage={false}
      isNew={false}
      changed={false}
      up={null}
      down={null}
      onEdit={() => openOption(opt.id)}
      onDuplicate={null}
      deleteBlockedBy={[]}
      onDelete={() => {}}
    />
  )

  return (
    <div className="grid gap-4">
      {anything ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-3">
            <Tabs value={view} onValueChange={(v) => setView(v as 'list' | 'map')}>
              <TabsList className="h-9">
                <TabsTrigger value="list" className="px-3">List</TabsTrigger>
                <TabsTrigger value="map" className="px-3">Map</TabsTrigger>
              </TabsList>
            </Tabs>
            <p className="text-sm text-muted-foreground">
              {everything.length} option{everything.length === 1 ? '' : 's'}, shown to customers in this order
              {rules.length + inheritedRules.length > 0
                ? `, and ${rules.length + inheritedRules.length} rule${rules.length + inheritedRules.length === 1 ? '' : 's'}`
                : ''}
              .
            </p>
          </div>
          <div className="flex items-center gap-2">
            {multilingual ? (
              <Select value={editLocale} onValueChange={setEditLocale}>
                <SelectTrigger size="sm" className="w-[9rem]" aria-label="Language">
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
            ) : null}
            {templateButton}
            {addButton}
          </div>
        </div>
      ) : null}

      {anything && owner.kind === 'product' ? (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          <span className="text-muted-foreground">On the product page</span>
          <Select value={layout} onValueChange={(v) => setLayoutDraft(v as ConfiguratorLayout)} disabled={!canManage}>
            <SelectTrigger size="sm" className="w-[10rem]" aria-label="Shown as">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="LIST">{LAYOUT_LABEL.LIST}</SelectItem>
              <SelectItem value="STEPS">{LAYOUT_LABEL.STEPS}</SelectItem>
            </SelectContent>
          </Select>
          <span className="text-xs text-muted-foreground">
            {layout === 'STEPS'
              ? 'One option at a time, with progress and the earlier picks above it.'
              : 'Every option at once, in this order.'}
          </span>
          {layoutChanged ? (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <span aria-hidden className="size-1.5 rounded-full bg-warning" />
              Unsaved
            </span>
          ) : null}
        </div>
      ) : null}

      {templateError ? (
        <Alert variant="destructive">
          <AlertDescription>{templateError}</AlertDescription>
        </Alert>
      ) : null}

      {!anything ? (
        <div className="flex flex-col items-center gap-3 rounded-md border border-dashed bg-muted/20 px-6 py-10 text-center">
          <div className="grid max-w-md gap-1">
            <p className="text-sm font-medium">No options yet</p>
            <p className="text-sm text-muted-foreground">
              {owner.kind === 'product'
                ? 'Options let customers personalise the product as they add it to the cart - a gift wrap choice, an engraving, a delivery date. Each can add to the price. A template brings a set kept in one place, and a product type can hand templates to all of its products.'
                : 'Add the options every product using this template should ask for, and rules over them. Edits reach every product using it.'}
            </p>
          </div>
          <div className="flex flex-wrap justify-center gap-2">
            {templateButton}
            {addButton}
          </div>
        </div>
      ) : view === 'map' ? (
        <Suspense fallback={<div className="h-[360px] animate-pulse rounded-md border bg-muted/30" />}>
          <OptionsMap
            options={owner.kind === 'product' ? everything : everything.map((o) => ({ ...o, templateName: null }))}
            rules={[...inheritedRules.map((r) => r.rule), ...rules]}
            locale={editLocale}
            defaultLocale={defaultLocale}
            currency={ctx.baseCurrency}
            onOpenOption={openOption}
            onOpenRule={(id) => {
              const fromTemplate = (subject.optionRules ?? []).find((r) => r.id === id && inheritedFrom(r.templateId))
              if (fromTemplate?.templateId) navigate(templatePath(fromTemplate.templateId))
              else if (canManage) setRuleEditor({ ruleId: id })
            }}
          />
        </Suspense>
      ) : (
        <div className="grid gap-3">
          {typeTemplates.map((t) => {
            const theirs = inherited.filter((o) => o.templateId === t.id)
            return (
              <TemplateGroup
                key={`type-${t.id}`}
                id={t.id}
                name={t.name}
                count={theirs.length}
                canManage={false}
                productType={owner.kind === 'product' && owner.productType ? owner.productType : { id: '', name: 'its type' }}
              >
                {theirs.map(inheritedRow)}
              </TemplateGroup>
            )
          })}
          {templates.map((t, i) => {
            const alsoFromType = typeIds.has(t.id)
            const theirs = inherited.filter((o) => o.templateId === t.id)
            const ids = templates.map((x) => x.id)
            const swap = (j: number) => {
              const next = [...ids]
              ;[next[i], next[j]] = [next[j], next[i]]
              return () => void setTemplates(next)
            }
            return (
              <TemplateGroup
                key={t.id}
                id={t.id}
                name={t.name}
                count={theirs.length}
                canManage={canManage}
                note={alsoFromType ? 'Its product type hands it over too, so its options show in the type\'s group above. Taking it off here changes nothing customers see.' : undefined}
                removeBlockedBy={leaningOn(t.id)}
                detachBlockedBy={alsoFromType ? 'It comes from the product type too' : dirty ? 'Save or discard your changes here first' : null}
                onMoveUp={i > 0 ? swap(i - 1) : null}
                onMoveDown={i < templates.length - 1 ? swap(i + 1) : null}
                onRemove={() => setConfirmTemplate({ kind: 'remove', id: t.id, name: t.name })}
                onDetach={() => setConfirmTemplate({ kind: 'detach', id: t.id, name: t.name })}
              >
                {theirs.map(inheritedRow)}
              </TemplateGroup>
            )
          })}
          {rows.length === 0 ? (
            <p className="rounded-md border border-dashed px-4 py-4 text-center text-xs text-muted-foreground">
              No options of its own - add one for anything only this product asks.
            </p>
          ) : (
            <ul className="divide-y rounded-md border" aria-label="Own options">
              {rows.map((opt) => {
                const index = sorted.findIndex((o) => o.id === opt.id)
                if (index < 0) {
                  return (
                    <li key={opt.id} className="flex items-center justify-between gap-3 bg-muted/30 px-4 py-3">
                      <span className="min-w-0 truncate text-sm text-muted-foreground line-through">{optionContentFor(opt, editLocale).title || 'Untitled option'}</span>
                      <span className="flex shrink-0 items-center gap-2">
                        <span className="text-xs text-destructive">Will remove</span>
                        {canManage ? (
                          <Button
                            type="button" variant="ghost" size="sm" className="h-7"
                            onClick={() => setPendingOps((prev) => prev.filter((op) => !(op.kind === 'delete-option' && op.id === opt.id)))}
                          >
                            <Undo2Icon className="size-3.5" />
                            Undo
                          </Button>
                        ) : null}
                      </span>
                    </li>
                  )
                }
                const above = sorted[index - 1]
                const below = sorted[index + 1]
                const upBlocked = !!above && blockedSwap(above, opt)
                const downBlocked = !!below && blockedSwap(opt, below)
                const blockers = usersOf({ optionId: opt.id })
                return (
                  <OptionRow
                    key={opt.id}
                    option={opt}
                    all={everything}
                    locale={editLocale}
                    currency={ctx.baseCurrency}
                    canManage={canManage}
                    isNew={isTemp(opt.id)}
                    changed={touched.has(opt.id)}
                    up={!above ? null : upBlocked ? `Its condition reads ${optionContentFor(above, editLocale).title || 'the option above'}` : () => move(index, index - 1)}
                    down={!below ? null : downBlocked ? `${optionContentFor(below, editLocale).title || 'The option below'} has a condition that reads this one` : () => move(index, index + 1)}
                    onEdit={() => setEditor({ optionId: opt.id })}
                    onDuplicate={isTemp(opt.id) ? null : () => void duplicateOptionMutation({ variables: { id: opt.id } })}
                    deleteBlockedBy={blockers}
                    onDelete={() => stage([{ kind: 'delete-option', id: opt.id }])}
                  />
                )
              })}
            </ul>
          )}
        </div>
      )}

      {anything && view === 'list' ? (
        <RulesList
          rules={rules}
          inherited={inheritedRules}
          saved={savedRules}
          options={everything}
          locale={editLocale}
          defaultLocale={defaultLocale}
          currency={ctx.baseCurrency}
          canManage={canManage}
          onAdd={() => setRuleEditor({ ruleId: null })}
          onEdit={(id) => setRuleEditor({ ruleId: id })}
          onDelete={(id) => setRulesDraft(rules.filter((r) => r.id !== id))}
          onMove={(from, to) => {
            const next = [...rules]
            next.splice(to, 0, ...next.splice(from, 1))
            setRulesDraft(next)
          }}
        />
      ) : null}

      {canManage ? (
        <OptionDialog
          open={editor !== null && (editor.optionId === null || editing !== null)}
          onOpenChange={(open) => !open && setEditor(null)}
          option={editing}
          options={everything}
          productId={owner.kind === 'product' ? owner.id : ''}
          refetchQueries={refetch}
          locale={editLocale}
          defaultLocale={defaultLocale}
          multilingual={multilingual}
          nextSortOrder={nextSortOrder}
          ctx={ctx}
          usedBy={(valueId) => usersOf({ valueId })}
          onDone={stage}
        />
      ) : null}
      {canManage ? (
        <RuleDialog
          open={ruleEditor !== null && (ruleEditor.ruleId === null || editingRule !== null)}
          onOpenChange={(open) => !open && setRuleEditor(null)}
          rule={editingRule}
          options={everything}
          locale={editLocale}
          defaultLocale={defaultLocale}
          multilingual={multilingual}
          ctx={ctx}
          onDone={(rule) =>
            setRulesDraft(rules.some((r) => r.id === rule.id) ? rules.map((r) => (r.id === rule.id ? rule : r)) : [...rules, rule])
          }
        />
      ) : null}
      {canManage && owner.kind === 'product' ? (
        <TemplatesDialog
          open={managing}
          onOpenChange={setManaging}
          usedIds={new Set(templates.map((t) => t.id))}
          inheritedIds={typeIds}
          onUse={async (id) => {
            await setTemplatesMutation({ variables: { productId: owner.id, templateIds: [...templates.map((t) => t.id), id] } })
          }}
          canCreate={allowedChannelScopes('catalog.manage').allChannels}
          defaultCurrency={subject.currency}
        />
      ) : null}
      <ConfirmDialog
        open={confirmTemplate !== null}
        onOpenChange={(open) => !open && setConfirmTemplate(null)}
        title={confirmTemplate?.kind === 'detach' ? `Detach ${confirmTemplate.name}?` : `Remove ${confirmTemplate?.name ?? ''}?`}
        description={
          confirmTemplate?.kind === 'detach'
            ? 'Its options and rules are copied onto this product as its own, and the product stops following the template. Changes to the template no longer reach it. This applies right away.'
            : confirmTemplate && typeIds.has(confirmTemplate.id)
              ? 'Its product type still hands it over, so its options keep showing on this product. Only the pick here goes.'
              : 'Its options stop showing on this product right away. The template itself stays as it is.'
        }
        confirmLabel={confirmTemplate?.kind === 'detach' ? 'Detach' : 'Remove'}
        destructive={confirmTemplate?.kind === 'remove'}
        onConfirm={async () => {
          if (!confirmTemplate) return
          if (confirmTemplate.kind === 'detach') {
            await detachMutation({ variables: { productId: owner.id, templateId: confirmTemplate.id } })
          } else {
            await setTemplatesMutation({ variables: { productId: owner.id, templateIds: templates.filter((t) => t.id !== confirmTemplate.id).map((t) => t.id) } })
          }
          setConfirmTemplate(null)
        }}
      />
    </div>
  )
}

function OptionRow({
  option,
  all,
  locale,
  currency,
  canManage,
  isNew,
  changed,
  up,
  down,
  onEdit,
  onDuplicate,
  deleteBlockedBy,
  onDelete,
}: {
  option: ProductOption
  all: ProductOption[]
  locale: string
  currency: string
  canManage: boolean
  isNew: boolean
  changed: boolean
  // null at the end of the list, a reason string when a condition forbids the move
  up: (() => void) | string | null
  down: (() => void) | string | null
  onEdit: () => void
  onDuplicate: (() => void) | null
  deleteBlockedBy: string[]
  onDelete: () => void
}) {
  const content = optionContentFor(option, locale)
  const info = INPUT_TYPES[option.inputType]
  const title = content.title || 'Untitled option'
  const condition = conditionSummary(option, all, locale)
  const values = [...option.values].sort((a, b) => a.sortOrder - b.sortOrder)
  const extra = priceLabel(option.priceValue, option.priceType, currency)
  const limits = [
    option.maxChars ? `Up to ${option.maxChars} characters` : null,
    option.fileExtensions ? option.fileExtensions.split(',').map((x) => x.trim().toUpperCase()).join(', ') : null,
    option.maxFileBytes ? `up to ${Number((option.maxFileBytes / (1024 * 1024)).toFixed(1))} MB` : null,
  ].filter(Boolean)

  const arrow = (dir: 'up' | 'down', action: (() => void) | string | null) => (
    <span title={typeof action === 'string' ? action : undefined}>
      <Button
        type="button" variant="ghost" size="icon" className="size-7" disabled={typeof action !== 'function'}
        aria-label={`Move ${title} ${dir}`} onClick={typeof action === 'function' ? action : undefined}
      >
        {dir === 'up' ? <ArrowUpIcon className="size-3.5" /> : <ArrowDownIcon className="size-3.5" />}
      </Button>
    </span>
  )

  return (
    <li className="flex items-start gap-2 px-3 py-3 sm:gap-3 sm:px-4">
      {canManage ? <div className="flex flex-col">{arrow('up', up)}{arrow('down', down)}</div> : null}
      {option.image ? (
        <img src={option.image.url} alt="" className="mt-0.5 size-8 shrink-0 rounded-md border object-cover" />
      ) : (
        <info.Icon className="mt-1 size-4 shrink-0 text-muted-foreground" />
      )}
      <div className="grid min-w-0 flex-1 gap-1.5">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          {canManage ? (
            <button type="button" onClick={onEdit} className="text-left font-medium hover:underline">{title}</button>
          ) : (
            <span className="font-medium">{title}</span>
          )}
          <span className="text-xs text-muted-foreground">{info.label}</span>
          {option.isRequired ? <Badge variant="secondary" className="font-normal">Required</Badge> : null}
          {isNew ? (
            <span className="text-xs text-success">New</span>
          ) : changed ? (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <span aria-hidden className="size-1.5 rounded-full bg-warning" />
              Unsaved changes
            </span>
          ) : null}
        </div>
        {content.helpText ? <p className="text-xs text-muted-foreground">{content.helpText}</p> : null}
        {isChoice(option.inputType) ? (
          values.length === 0 ? (
            <p className="text-xs text-warning">No choices yet - customers have nothing to pick.</p>
          ) : (
            <ul className="flex flex-wrap gap-1.5" aria-label={`${title} choices`}>
              {values.slice(0, MAX_CHIPS).map((v) => {
                const price = v.sourceProduct ? null : priceLabel(v.priceValue, v.priceType, currency)
                return (
                  <li key={v.id} className="inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-0.5 text-xs">
                    {v.image ? <img src={v.image.url} alt="" className="-ml-1 size-4 shrink-0 rounded-sm object-cover" /> : null}
                    {v.sourceProduct ? <LinkIcon className="size-3 shrink-0 text-muted-foreground" aria-label="Linked product" /> : null}
                    <span className="truncate">{valueContentFor(v, locale) || 'Untitled choice'}</span>
                    {price ? <span className="text-muted-foreground">{price}</span> : null}
                    {v.isDefault ? <span className="text-muted-foreground">· pre-selected</span> : null}
                  </li>
                )
              })}
              {values.length > MAX_CHIPS ? <li className="px-1 text-xs text-muted-foreground">+{values.length - MAX_CHIPS} more</li> : null}
            </ul>
          )
        ) : limits.length > 0 || extra ? (
          <p className="text-xs text-muted-foreground">{[...limits, extra].filter(Boolean).join(' · ')}</p>
        ) : null}
        {condition ? (
          <p className="flex items-start gap-1 text-xs text-muted-foreground">
            <EyeIcon className="mt-0.5 size-3 shrink-0" />
            {condition}
          </p>
        ) : null}
      </div>
      {canManage ? (
        <div className="flex shrink-0 items-center gap-1">
          <Button type="button" variant="ghost" size="icon" className="size-8" aria-label={`Edit ${title}`} onClick={onEdit}>
            <PencilIcon className="size-3.5" />
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="ghost" size="icon" className="size-8" aria-label={`More actions for ${title}`}>
                <EllipsisIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64">
              <DropdownMenuItem disabled={!onDuplicate} onSelect={() => onDuplicate?.()}>
                <CopyIcon className="size-4" />
                <span className="grid">
                  Duplicate
                  <span className="text-xs text-muted-foreground">{onDuplicate ? 'Adds the copy right away' : 'Save the page first'}</span>
                </span>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" disabled={deleteBlockedBy.length > 0} onSelect={onDelete}>
                <Trash2Icon className="size-4" />
                <span className="grid">
                  Delete option
                  {deleteBlockedBy.length > 0 ? (
                    <span className="text-xs text-muted-foreground">Used by {deleteBlockedBy.join(', ')}</span>
                  ) : null}
                </span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : null}
    </li>
  )
}
