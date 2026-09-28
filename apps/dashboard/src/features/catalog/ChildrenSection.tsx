import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  AlertCircleIcon,
  CheckCircle2Icon,
  PlusIcon,
  SearchIcon,
  SparklesIcon,
  Trash2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatCents } from '@/lib/format'
import { dollarsToCents } from '@/shared/schemas/fields'
import {
  AddProductChildDocument,
  GenerateConfigurableChildrenDocument,
  ProductDocument,
  ProductsDocument,
  RemoveProductChildDocument,
} from './Catalog.gql'

type ChildProduct = {
  id: string
  url: string
  name?: string | null
  sku?: string | null
  defaultPrice: number
  defaultCurrencyCode: string
}

type ConfigurableOption = {
  id: string
  slug: string
  name: string
}

type AttributeValue = {
  id: string
  slug: string
  name: string
  value: string
  sortOrder?: number | null
}

type ProductTypeAttributeShape = {
  attribute: {
    id: string
    slug: string
    name: string
    values: AttributeValue[]
  }
}

type ParentForChildren = {
  id: string
  sku?: string | null
  defaultPrice: number
  defaultCurrencyCode: string
  contentLocale?: string | null
  children: ChildProduct[]
  configurableOptions: ConfigurableOption[]
  productType: {
    id: string
    attributes: ProductTypeAttributeShape[]
  }
}

type CandidateAttributeRow = {
  attribute: { id: string; slug: string; name: string }
  selectedValues: Array<{ id: string; slug: string; name: string; value: string }>
}

type CandidateProduct = {
  id: string
  name?: string | null
  sku?: string | null
  defaultPrice: number
  defaultCurrencyCode: string
  productType: { id: string }
  attributeValues: CandidateAttributeRow[]
}

export function ChildrenSection({
  product,
  productTypeId,
  canManage,
  embedded = false,
}: {
  product: ParentForChildren
  productTypeId: string
  canManage: boolean
  embedded?: boolean
}) {
  const [sheetOpen, setSheetOpen] = useState(false)
  const [confirmingRemove, setConfirmingRemove] = useState<ChildProduct | null>(null)
  const [removeError, setRemoveError] = useState<string | null>(null)

  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [removeChild, { loading: removing }] = useMutation(RemoveProductChildDocument, {
    refetchQueries: refetch,
  })

  async function onConfirmRemove() {
    if (!confirmingRemove) return
    setRemoveError(null)
    try {
      await removeChild({ variables: { parentId: product.id, childId: confirmingRemove.id } })
      setConfirmingRemove(null)
    } catch (e) {
      setRemoveError(e instanceof Error ? e.message : 'Remove failed.')
    }
  }

  // Look up each option attribute's full value set from the product type's attribute list (configurableOptions only
  // gives id/slug/name without the values, but the type-level attribute carries them)
  const optionsWithValues = useMemo(() => {
    return product.configurableOptions
      .map((opt) => {
        const fromType = product.productType.attributes.find(
          (a) => a.attribute.id === opt.id,
        )
        return {
          ...opt,
          values: (fromType?.attribute.values ?? []).slice().sort(
            (a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0),
          ),
        }
      })
  }, [product.configurableOptions, product.productType.attributes])

  const body = (
    <>
      {!embedded && (
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="grid gap-1">
              <CardTitle className="text-base">Child products</CardTitle>
              <CardDescription>
                {product.configurableOptions.length > 0
                  ? `Children must have values for: ${product.configurableOptions.map((o) => o.name).join(', ')}.`
                  : 'Each child is a full product with its own SKU, price, and attributes.'}
              </CardDescription>
            </div>
            {canManage && (
              <Button size="sm" onClick={() => setSheetOpen(true)}>
                <PlusIcon className="size-4" />
                Add children
              </Button>
            )}
          </div>
        </CardHeader>
      )}

      {embedded && (
        <div className="flex items-center justify-between">
          <div className="grid gap-1">
            <h3 className="text-sm font-medium">Child products</h3>
            <p className="text-sm text-muted-foreground">
              {product.configurableOptions.length > 0
                ? `Children must have values for: ${product.configurableOptions.map((o) => o.name).join(', ')}.`
                : 'Each child is a full product with its own SKU, price, and attributes.'}
            </p>
          </div>
          {canManage && (
            <Button size="sm" onClick={() => setSheetOpen(true)}>
              <PlusIcon className="size-4" />
              Add children
            </Button>
          )}
        </div>
      )}

      <div className={embedded ? 'mt-3' : 'px-6 pb-6'}>
        {product.children.length === 0 ? (
          <p className="text-sm text-muted-foreground">No child products yet.</p>
        ) : (
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>SKU</TableHead>
                  {product.configurableOptions.length > 0 && (
                    <TableHead>Options</TableHead>
                  )}
                  <TableHead className="w-28">Default price</TableHead>
                  {canManage && <TableHead className="w-12" />}
                </TableRow>
              </TableHeader>
              <TableBody>
                {product.children.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell>
                      <Link to={`/products/${c.id}`} className="font-medium hover:underline">
                        {c.name ?? c.sku ?? ''}
                      </Link>
                    </TableCell>
                    <TableCell>
                      {c.sku ? (
                        <code className="font-mono text-xs">{c.sku}</code>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    {product.configurableOptions.length > 0 && (
                      <TableCell>
                        <Link to={`/products/${c.id}`} className="text-xs text-muted-foreground hover:underline">
                          view values →
                        </Link>
                      </TableCell>
                    )}
                    <TableCell className="font-mono text-xs">
                      {formatCents(c.defaultPrice, c.defaultCurrencyCode)}
                    </TableCell>
                    {canManage && (
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-7"
                          onClick={() => { setRemoveError(null); setConfirmingRemove(c) }}
                          aria-label={`Remove ${c.name ?? c.sku ?? ''}`}
                        >
                          <Trash2Icon className="size-3.5" />
                        </Button>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {canManage && (
        <AddChildSheet
          parent={product}
          optionsWithValues={optionsWithValues}
          productTypeId={productTypeId}
          open={sheetOpen}
          onOpenChange={setSheetOpen}
          refetch={refetch}
        />
      )}

      <AlertDialog
        open={!!confirmingRemove}
        onOpenChange={(v) => { if (!v) { setConfirmingRemove(null); setRemoveError(null) } }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove {confirmingRemove?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This unlinks the child from this configurable product. The child product itself is not deleted.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {removeError && (
            <Alert variant="destructive">
              <AlertTitle>Remove failed</AlertTitle>
              <AlertDescription>{removeError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={removing}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={removing}
              onClick={(e) => { e.preventDefault(); onConfirmRemove() }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {removing ? 'Removing…' : 'Remove child'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )

  if (embedded) return body
  return <Card>{body}</Card>
}

type OptionWithValues = ConfigurableOption & { values: AttributeValue[] }

function AddChildSheet({
  parent,
  optionsWithValues,
  productTypeId,
  open,
  onOpenChange,
  refetch,
}: {
  parent: ParentForChildren
  optionsWithValues: OptionWithValues[]
  productTypeId: string
  open: boolean
  onOpenChange: (v: boolean) => void
  refetch: Array<{ query: typeof ProductDocument; variables: { id: string } }>
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        className="flex w-[96vw] flex-col gap-0 p-0 sm:!max-w-[min(96vw,1400px)]"
        side="right"
      >
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>Add child products</SheetTitle>
          <SheetDescription>
            Link existing same-type products, or generate brand-new variants from option combinations.
          </SheetDescription>
        </SheetHeader>

        <Tabs defaultValue="link" className="flex min-h-0 flex-1 flex-col">
          <div className="border-b px-6 pt-3">
            <TabsList>
              <TabsTrigger value="link">Link existing</TabsTrigger>
              <TabsTrigger value="generate" disabled={optionsWithValues.length === 0}>
                <SparklesIcon className="size-3.5" />
                Generate variants
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="link" className="flex min-h-0 flex-1 flex-col data-[state=inactive]:hidden">
            <LinkExistingTab
              parent={parent}
              productTypeId={productTypeId}
              optionsWithValues={optionsWithValues}
              refetch={refetch}
              onDone={() => onOpenChange(false)}
            />
          </TabsContent>

          <TabsContent value="generate" className="flex min-h-0 flex-1 flex-col data-[state=inactive]:hidden">
            <GenerateVariantsTab
              parent={parent}
              optionsWithValues={optionsWithValues}
              refetch={refetch}
              onDone={() => onOpenChange(false)}
            />
          </TabsContent>
        </Tabs>
      </SheetContent>
    </Sheet>
  )
}

// ──────────────────────── Link existing tab ────────────────────────

function LinkExistingTab({
  parent,
  productTypeId,
  optionsWithValues,
  refetch,
  onDone,
}: {
  parent: ParentForChildren
  productTypeId: string
  optionsWithValues: OptionWithValues[]
  refetch: Array<{ query: typeof ProductDocument; variables: { id: string } }>
  onDone: () => void
}) {
  const { data, loading } = useQuery(ProductsDocument)
  const [addChild, { loading: linking }] = useMutation(AddProductChildDocument, { refetchQueries: refetch })

  const [selectedValues, setSelectedValues] = useState<Record<string, Set<string>>>({})
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [globalError, setGlobalError] = useState<string | null>(null)
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({})
  const [search, setSearch] = useState('')

  const excludeIds = useMemo(
    () => new Set<string>([parent.id, ...parent.children.map((c) => c.id)]),
    [parent.id, parent.children],
  )

  const candidates: CandidateProduct[] = useMemo(() => {
    const all = (data?.products ?? []) as CandidateProduct[]
    const eligible = all.filter((p) => !excludeIds.has(p.id) && p.productType.id === productTypeId)
    // Sort by display name so the long list is scannable
    return eligible.slice().sort((a, b) => (a.name ?? a.sku ?? '').localeCompare(b.name ?? b.sku ?? ''))
  }, [data, excludeIds, productTypeId])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return candidates.filter((c) => {
      if (q) {
        const haystack = [c.name ?? '', c.sku ?? ''].join(' ').toLowerCase()
        if (!haystack.includes(q)) return false
      }
      return optionsWithValues.every((opt) => {
        const want = selectedValues[opt.id]
        if (!want || want.size === 0) return true
        const row = c.attributeValues.find((av) => av.attribute.id === opt.id)
        if (!row) return false
        return row.selectedValues.some((sv) => want.has(sv.id))
      })
    })
  }, [candidates, optionsWithValues, selectedValues, search])

  function toggleValue(optId: string, valueId: string) {
    setSelectedValues((prev) => {
      const next = { ...prev }
      const set = new Set(next[optId] ?? [])
      if (set.has(valueId)) set.delete(valueId)
      else set.add(valueId)
      if (set.size === 0) delete next[optId]
      else next[optId] = set
      return next
    })
  }

  function togglePicked(id: string) {
    setPicked((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function clearAllFilters() {
    setSelectedValues({})
  }

  async function linkSelected() {
    setGlobalError(null)
    setRowErrors({})
    const ids = Array.from(picked)
    let succeeded = 0
    const nextErrors: Record<string, string> = {}
    for (const id of ids) {
      try {
        await addChild({ variables: { parentId: parent.id, childId: id, sortOrder: parent.children.length + succeeded } })
        succeeded++
      } catch (e) {
        nextErrors[id] = e instanceof Error ? e.message : 'Link failed.'
      }
    }
    setRowErrors(nextErrors)
    if (succeeded > 0) {
      const linked = new Set(Object.keys(nextErrors))
      setPicked((prev) => new Set(Array.from(prev).filter((id) => linked.has(id))))
    }
    if (Object.keys(nextErrors).length === 0) {
      onDone()
    } else if (succeeded === 0) {
      setGlobalError('Nothing was linked. Fix the errors below and try again.')
    } else {
      setGlobalError(`Linked ${succeeded} of ${ids.length}. The rows below failed - fix and retry.`)
    }
  }

  // Header checkbox semantics: checked when every visible row is picked, indeterminate when some-but-not-all are picked - Clicking it
  // toggles all visible (filtered) rows; selections in the picked-set that aren't in the current filter are preserved
  const allFilteredPicked = filtered.length > 0 && filtered.every((c) => picked.has(c.id))
  const someFilteredPicked = filtered.some((c) => picked.has(c.id))
  function toggleAllVisible(check: boolean) {
    setPicked((prev) => {
      const next = new Set(prev)
      for (const c of filtered) {
        if (check) next.add(c.id)
        else next.delete(c.id)
      }
      return next
    })
  }

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[260px_1fr]">
      <aside className="flex min-h-0 flex-col border-r bg-muted/20">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Filters</span>
          {Object.keys(selectedValues).length > 0 && (
            <button
              type="button"
              className="text-xs text-muted-foreground hover:text-foreground hover:underline"
              onClick={clearAllFilters}
            >
              Clear
            </button>
          )}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-4 text-sm">
          {optionsWithValues.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No configurable options on this parent yet - set them up first to filter candidates by option values.
            </p>
          ) : (
            <div className="grid gap-4">
              {optionsWithValues.map((opt) => (
                <div key={opt.id} className="grid gap-2">
                  <div className="text-xs font-medium">{opt.name}</div>
                  <div className="grid gap-1.5">
                    {opt.values.length === 0 ? (
                      <p className="text-xs text-muted-foreground">No values defined.</p>
                    ) : (
                      opt.values.map((v) => {
                        const checked = selectedValues[opt.id]?.has(v.id) ?? false
                        return (
                          <label
                            key={v.id}
                            className="flex cursor-pointer items-center gap-2 text-xs"
                          >
                            <Checkbox
                              checked={checked}
                              onCheckedChange={() => toggleValue(opt.id, v.id)}
                            />
                            <span>{v.name || v.value}</span>
                          </label>
                        )
                      })
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </aside>

      <div className="flex min-h-0 flex-1 flex-col">
        <div className="grid gap-2 border-b px-4 py-3">
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search name, slug, or SKU…"
              className="h-9 pl-8"
            />
          </div>
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {loading ? 'Loading…' : `Showing ${filtered.length} of ${candidates.length}`}
            </span>
            <span>
            {picked.size > 0 ? `${picked.size} selected` : ' '}
            </span>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {filtered.length === 0 && !loading ? (
            <p className="p-6 text-sm text-muted-foreground">
              {candidates.length === 0
                ? 'No eligible products of this type - create some simple products first, or use the Generate tab.'
                : 'No candidates match the current filters.'}
            </p>
          ) : (
            <Table>
              <TableHeader className="sticky top-0 z-10 bg-popover">
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      checked={allFilteredPicked}
                      onCheckedChange={() => toggleAllVisible(!allFilteredPicked)}
                      aria-label={allFilteredPicked ? 'Deselect all visible' : 'Select all visible'}
                      title={
                        someFilteredPicked && !allFilteredPicked
                          ? `${filtered.filter((c) => picked.has(c.id)).length} of ${filtered.length} selected`
                          : undefined
                      }
                    />
                  </TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>SKU</TableHead>
                  {optionsWithValues.map((opt) => (
                    <TableHead key={opt.id}>{opt.name}</TableHead>
                  ))}
                  <TableHead className="w-28">Default price</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((c) => {
                  const error = rowErrors[c.id]
                  return (
                    <TableRow
                      key={c.id}
                      className={error ? 'bg-destructive/5' : ''}
                      onClick={() => togglePicked(c.id)}
                    >
                      <TableCell>
                        <Checkbox
                          checked={picked.has(c.id)}
                          onCheckedChange={() => togglePicked(c.id)}
                          onClick={(e) => e.stopPropagation()}
                          aria-label={`Pick ${c.name ?? c.sku ?? ''}`}
                        />
                      </TableCell>
                      <TableCell>
                        <div className="grid">
                          <span className="font-medium">{c.name ?? c.sku ?? ''}</span>
                          {error && (
                            <span className="flex items-center gap-1 text-xs text-destructive">
                              <AlertCircleIcon className="size-3" />
                              {error}
                            </span>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        {c.sku ? (
                          <code className="font-mono text-xs">{c.sku}</code>
                        ) : (
                          <span className="text-xs text-muted-foreground">-</span>
                        )}
                      </TableCell>
                      {optionsWithValues.map((opt) => {
                        const row = c.attributeValues.find((av) => av.attribute.id === opt.id)
                        const values = row?.selectedValues ?? []
                        return (
                          <TableCell key={opt.id}>
                            {values.length === 0 ? (
                              <span className="text-xs text-muted-foreground">-</span>
                            ) : (
                              <div className="flex flex-wrap gap-1">
                                {values.map((v) => (
                                  <Badge key={v.id} variant="outline" className="font-normal">
                                    {v.name || v.value}
                                  </Badge>
                                ))}
                              </div>
                            )}
                          </TableCell>
                        )
                      })}
                      <TableCell className="font-mono text-xs">
                        {formatCents(c.defaultPrice, c.defaultCurrencyCode)}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </div>

        {globalError && (
          <Alert variant="destructive" className="m-4">
            <AlertTitle>Some links failed</AlertTitle>
            <AlertDescription>{globalError}</AlertDescription>
          </Alert>
        )}

        <SheetFooter className="border-t px-4 py-3">
          <div className="flex items-center justify-end gap-2">
            <Button variant="outline" onClick={onDone}>Cancel</Button>
            <Button onClick={linkSelected} disabled={picked.size === 0 || linking}>
              {linking ? 'Linking…' : `Link ${picked.size || ''} product${picked.size === 1 ? '' : 's'}`.trim()}
            </Button>
          </div>
        </SheetFooter>
      </div>
    </div>
  )
}

// ──────────────────────── Generate variants tab ────────────────────────

type VariantRow = {
  key: string // composite of value IDs for stable React keys
  attributeValueIds: string[]
  optionDisplay: string[]
  name: string
  sku: string
  priceDollars: string
}

function GenerateVariantsTab({
  parent,
  optionsWithValues,
  refetch,
  onDone,
}: {
  parent: ParentForChildren
  optionsWithValues: OptionWithValues[]
  refetch: Array<{ query: typeof ProductDocument; variables: { id: string } }>
  onDone: () => void
}) {
  const [generate, { loading: generating }] = useMutation(GenerateConfigurableChildrenDocument, {
    refetchQueries: refetch,
  })

  const [chosenValues, setChosenValues] = useState<Record<string, Set<string>>>({})
  const [overrides, setOverrides] = useState<Record<string, Partial<VariantRow>>>({})
  const [error, setError] = useState<string | null>(null)

  function toggleValue(optId: string, valueId: string) {
    setChosenValues((prev) => {
      const next = { ...prev }
      const set = new Set(next[optId] ?? [])
      if (set.has(valueId)) set.delete(valueId)
      else set.add(valueId)
      if (set.size === 0) delete next[optId]
      else next[optId] = set
      return next
    })
  }

  function selectAllForOption(optId: string) {
    const all = optionsWithValues.find((o) => o.id === optId)?.values ?? []
    setChosenValues((prev) => ({ ...prev, [optId]: new Set(all.map((v) => v.id)) }))
  }

  function clearOption(optId: string) {
    setChosenValues((prev) => {
      const next = { ...prev }
      delete next[optId]
      return next
    })
  }

  // The combination matrix is fully derived from the chosen values
  const rows = useMemo<VariantRow[]>(() => {
    if (optionsWithValues.some((opt) => !chosenValues[opt.id] || chosenValues[opt.id]!.size === 0)) {
      return []
    }
    const perOption = optionsWithValues.map((opt) => {
      const chosenSet = chosenValues[opt.id]!
      return opt.values.filter((v) => chosenSet.has(v.id))
    })
    const combos: AttributeValue[][] = perOption.reduce<AttributeValue[][]>(
      (acc, values) => acc.flatMap((row) => values.map((v) => [...row, v])),
      [[]],
    )
    const parentSkuBase = (parent.sku ?? 'parent').trim() || 'parent'
    // The parent's user-facing name lives on translations; fall back to its SKU for the variant name template - The
    // child's URL is generated server-side from the name (or product-type template)
    const parentNameBase = parent.sku ?? 'variant'
    return combos.map((combo) => {
      const valueSlugs = combo.map((v) => v.slug)
      const valueNames = combo.map((v) => v.name || v.value)
      const sku = [parentSkuBase, ...valueSlugs.map((s) => s.toUpperCase())].join('-')
      const name = [parentNameBase, ...valueNames].join(' / ')
      return {
        key: combo.map((v) => v.id).join('|'),
        attributeValueIds: combo.map((v) => v.id),
        optionDisplay: valueNames,
        name,
        sku,
        priceDollars: (parent.defaultPrice / 100).toFixed(2),
      }
    })
  }, [chosenValues, optionsWithValues, parent.defaultPrice, parent.sku])

  function patchRow(key: string, patch: Partial<VariantRow>) {
    setOverrides((prev) => ({ ...prev, [key]: { ...prev[key], ...patch } }))
  }

  function effectiveRow(row: VariantRow): VariantRow {
    return { ...row, ...overrides[row.key] }
  }

  async function onGenerate() {
    setError(null)
    if (rows.length === 0) return
    const effective = rows.map(effectiveRow)
    for (const r of effective) {
      if (!r.name.trim()) {
        setError(`Variant ${r.optionDisplay.join(' / ')}: name is required.`)
        return
      }
    }

    try {
      await generate({
        variables: {
          parentId: parent.id,
          variants: effective.map((r, idx) => ({
            attributeValueIds: r.attributeValueIds,
            name: r.name,
            sku: r.sku || null,
            defaultPrice: dollarsToCents(r.priceDollars),
            defaultCurrencyCode: parent.defaultCurrencyCode,
            locale: parent.contentLocale ?? null,
            sortOrder: parent.children.length + idx,
          })),
        },
      })
      onDone()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Generate failed.')
    }
  }

  const totalCount = rows.length
  const allChosen = optionsWithValues.length > 0 &&
    optionsWithValues.every((opt) => chosenValues[opt.id] && chosenValues[opt.id]!.size > 0)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="grid gap-3 border-b px-6 py-4">
        <div className="grid gap-1">
          <h4 className="text-sm font-medium">Pick values per option</h4>
          <p className="text-xs text-muted-foreground">
            Every selected combination becomes a new simple product, linked under this configurable parent.
          </p>
        </div>
        {optionsWithValues.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            This parent has no configurable options yet. Define them in the section above before generating variants.
          </p>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {optionsWithValues.map((opt) => {
              const set = chosenValues[opt.id] ?? new Set<string>()
              return (
                <div key={opt.id} className="rounded-md border p-3">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-xs font-medium">{opt.name}</span>
                    <div className="flex gap-1 text-[10px] text-muted-foreground">
                      <button
                        type="button"
                        className="hover:underline"
                        onClick={() => selectAllForOption(opt.id)}
                      >
                        all
                      </button>
                      <span>·</span>
                      <button
                        type="button"
                        className="hover:underline"
                        onClick={() => clearOption(opt.id)}
                      >
                        none
                      </button>
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {opt.values.length === 0 ? (
                      <span className="text-xs text-muted-foreground">No values defined.</span>
                    ) : (
                      opt.values.map((v) => {
                        const checked = set.has(v.id)
                        return (
                          <button
                            key={v.id}
                            type="button"
                            onClick={() => toggleValue(opt.id, v.id)}
                            className={[
                              'rounded-md border px-2 py-1 text-xs transition',
                              checked
                                ? 'border-primary bg-primary/10 text-primary'
                                : 'hover:bg-accent/40',
                            ].join(' ')}
                          >
                            {v.name || v.value}
                          </button>
                        )
                      })
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="flex min-h-0 flex-1 flex-col">
        <div className="flex items-center justify-between border-b px-6 py-3">
          <span className="text-sm font-medium">Preview</span>
          <span className="text-xs text-muted-foreground">
            {totalCount > 0
              ? `${totalCount} variant${totalCount === 1 ? '' : 's'} will be created`
              : allChosen
                ? 'No combinations'
                : 'Pick at least one value per option to preview'}
          </span>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {totalCount === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">Nothing to preview yet.</p>
          ) : (
            <Table>
              <TableHeader className="sticky top-0 bg-popover">
                <TableRow>
                  <TableHead>Options</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>SKU</TableHead>
                  <TableHead className="w-32">Price ({parent.defaultCurrencyCode})</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => {
                  const eff = effectiveRow(row)
                  return (
                    <TableRow key={row.key}>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {row.optionDisplay.map((label) => (
                            <Badge key={label} variant="outline" className="font-normal">{label}</Badge>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Input
                          className="h-7"
                          value={eff.name}
                          onChange={(e) => patchRow(row.key, { name: e.target.value })}
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          className="h-7 font-mono text-xs"
                          value={eff.sku}
                          onChange={(e) => patchRow(row.key, { sku: e.target.value })}
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          className="h-7 font-mono text-xs"
                          type="number"
                          step="0.01"
                          min="0"
                          value={eff.priceDollars}
                          onChange={(e) => patchRow(row.key, { priceDollars: e.target.value })}
                        />
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </div>

        {error && (
          <Alert variant="destructive" className="m-4">
            <AlertTitle>Generate failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <SheetFooter className="border-t px-4 py-3">
          <div className="flex items-center justify-end gap-2">
            <Button variant="outline" onClick={onDone}>Cancel</Button>
            <Button onClick={onGenerate} disabled={totalCount === 0 || generating}>
              <CheckCircle2Icon className="size-4" />
              {generating ? 'Generating…' : `Generate ${totalCount || ''} variant${totalCount === 1 ? '' : 's'}`.trim()}
            </Button>
          </div>
        </SheetFooter>
      </div>
    </div>
  )
}

