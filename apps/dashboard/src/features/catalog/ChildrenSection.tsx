import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import {
  CheckCircle2Icon,
  PlusIcon,
  SparklesIcon,
  Trash2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import type { ColumnDef } from '@/components/data-table/tanstack'
import { ProductPicker, type PickerProduct } from './ProductPickerDialog'
import { dollarsToCents } from '@/shared/schemas/fields'
import {
  AddProductChildDocument,
  GenerateConfigurableChildrenDocument,
  ProductDocument,
  RemoveProductChildDocument,
} from './Catalog.gql'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

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
    slug: string
    attributes: ProductTypeAttributeShape[]
  }
}

export function ChildrenSection({
  product,
  canManage,
  embedded = false,
}: {
  product: ParentForChildren
  canManage: boolean
  embedded?: boolean
}) {
  const [sheetOpen, setSheetOpen] = useState(false)
  const [confirmingRemove, setConfirmingRemove] = useState<ChildProduct | null>(null)

  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [removeChild] = useMutation(RemoveProductChildDocument, {
    refetchQueries: refetch,
  })

  async function onConfirmRemove() {
    if (!confirmingRemove) return
    await removeChild({ variables: { parentId: product.id, childId: confirmingRemove.id } })
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
                          onClick={() => setConfirmingRemove(c)}
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
          open={sheetOpen}
          onOpenChange={setSheetOpen}
          refetch={refetch}
        />
      )}

      <ConfirmDialog
        open={!!confirmingRemove}
        onOpenChange={(v) => { if (!v) setConfirmingRemove(null) }}
        title={<>Remove {confirmingRemove?.name}?</>}
        description="This unlinks the child from this configurable product. The child product itself is not deleted."
        confirmLabel="Remove child"
        busyLabel="Removing…"
        destructive
        onConfirm={onConfirmRemove}
      />
    </>
  )

  if (embedded) return body
  return <Card>{body}</Card>
}

type OptionWithValues = ConfigurableOption & { values: AttributeValue[] }

function AddChildSheet({
  parent,
  optionsWithValues,
  open,
  onOpenChange,
  refetch,
}: {
  parent: ParentForChildren
  optionsWithValues: OptionWithValues[]
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
  optionsWithValues,
  refetch,
  onDone,
}: {
  parent: ParentForChildren
  optionsWithValues: OptionWithValues[]
  refetch: Array<{ query: typeof ProductDocument; variables: { id: string } }>
  onDone: () => void
}) {
  const [addChild] = useMutation(AddProductChildDocument, { refetchQueries: refetch })
  const [selectedValues, setSelectedValues] = useState<Record<string, Set<string>>>({})

  const childIds = useMemo(() => new Set(parent.children.map((c) => c.id)), [parent.children])
  const selfId = useMemo(() => new Set([parent.id]), [parent.id])
  // The server matches AND across options and OR within one - the same rule this sidebar promises
  const attributeValueIds = useMemo(
    () => Object.values(selectedValues).flatMap((set) => [...set]),
    [selectedValues],
  )

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

  const optionColumns = useMemo<ColumnDef<PickerProduct>[]>(
    () =>
      optionsWithValues.map((opt) => ({
        id: `option:${opt.id}`,
        header: () => opt.name,
        enableSorting: false,
        cell: ({ row }) => {
          const values = row.original.attributeValues?.find((av) => av.attribute.id === opt.id)?.selectedValues ?? []
          if (values.length === 0) return <span className="text-xs text-muted-foreground">-</span>
          return (
            <div className="flex flex-wrap gap-1">
              {values.map((v) => (
                <Badge key={v.id} variant="outline" className="font-normal">
                  {v.name || v.value}
                </Badge>
              ))}
            </div>
          )
        },
      })),
    [optionsWithValues],
  )

  const sidebar = (
    <div className="grid gap-4 rounded-md border bg-muted/20 p-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Option values</span>
        {Object.keys(selectedValues).length > 0 && (
          <button
            type="button"
            className="text-xs text-muted-foreground hover:text-foreground hover:underline"
            onClick={() => setSelectedValues({})}
          >
            Clear
          </button>
        )}
      </div>
      {optionsWithValues.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          No configurable options on this parent yet - set them up first to filter candidates by option values.
        </p>
      ) : (
        optionsWithValues.map((opt) => (
          <div key={opt.id} className="grid gap-2">
            <div className="text-xs font-medium">{opt.name}</div>
            <div className="grid gap-1.5">
              {opt.values.length === 0 ? (
                <p className="text-xs text-muted-foreground">No values defined.</p>
              ) : (
                opt.values.map((v) => (
                  <label key={v.id} className="flex cursor-pointer items-center gap-2 text-xs">
                    <Checkbox
                      checked={selectedValues[opt.id]?.has(v.id) ?? false}
                      onCheckedChange={() => toggleValue(opt.id, v.id)}
                    />
                    <span>{v.name || v.value}</span>
                  </label>
                ))
              )}
            </div>
          </div>
        ))
      )}
    </div>
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col px-6 py-4">
      <ProductPicker
        takenIds={childIds}
        takenLabel="Already a child"
        excludeIds={selfId}
        productTypeSlugs={[parent.productType.slug]}
        attributeValueIds={attributeValueIds}
        withAttributes
        sidebar={sidebar}
        columns={optionColumns}
        showPrice
        emptyMessage="No products of this type match - create some simple products first, or use the Generate tab."
        confirmLabel={(n) => (n === 0 ? 'Link products' : `Link ${n} product${n === 1 ? '' : 's'}`)}
        onCancel={onDone}
        onDone={onDone}
        // Linking is this sheet's own explicit action, so it writes here - one at a time, so each
        // child takes the next sort slot, with failures kept ticked and explained per row
        onConfirm={async (picks) => {
          const rowErrors: Record<string, string> = {}
          let linked = 0
          for (const { product } of picks) {
            try {
              await addChild({
                variables: { parentId: parent.id, childId: product.id, sortOrder: parent.children.length + linked },
              })
              linked++
            } catch (e) {
              rowErrors[product.id] = e instanceof Error ? e.message : 'Link failed.'
            }
          }
          const failed = Object.keys(rowErrors).length
          if (failed === 0) return
          return {
            rowErrors,
            error:
              linked === 0
                ? 'Nothing was linked. Fix the errors below and try again.'
                : `Linked ${linked} of ${picks.length}. The rows below failed - fix and retry.`,
          }
        }}
      />
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

