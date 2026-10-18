import { useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { PlusIcon, StarIcon, Trash2Icon, Undo2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import {
  AddProductToCategoryDocument,
  ProductDocument,
  RemoveProductFromCategoryDocument,
  SetProductPrimaryCategoryDocument,
} from '../Catalog.gql'
import { CategoryPickerDialog } from '../category-picker/CategoryPickerDialog'
import { PATH_SEPARATOR, formatPath, pathOf, useCategoryIndex } from '../category-picker/category-options'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import {
  PRODUCT_CATEGORY_LABELS,
  productCategoriesDiff,
  productCategoriesSchema,
  productCategoryChanges,
  toggleCategory,
  toProductCategoryValues,
} from '../schemas/product'
import type { ProductCategoryItem } from './shared'

// ---------- Product categories ----------

// The categories a product sits in, listed by path with the primary starred. The dialog's picks and
// the row removals only stage; the page-header Save runs the deltas
export function ProductCategoriesPicker({
  productId,
  categories,
  primaryCategory,
  canManage,
  productVariables,
}: {
  productId: string
  categories: ProductCategoryItem[]
  primaryCategory: ProductCategoryItem | null
  canManage: boolean
  productVariables: { id: string }
}) {
  const [pickerOpen, setPickerOpen] = useState(false)
  const { index } = useCategoryIndex()

  const refetchQueries = [{ query: ProductDocument, variables: productVariables }]
  const [addToCategory] = useMutation(AddProductToCategoryDocument, { refetchQueries })
  const [removeFromCategory] = useMutation(RemoveProductFromCategoryDocument, { refetchQueries })
  const [setPrimary] = useMutation(SetProductPrimaryCategoryDocument, { refetchQueries })

  // The product's own rows name a category the picker list may not hold yet (still loading)
  const ownNames = useMemo(() => new Map(categories.map((c) => [c.id, c.name ?? c.slug])), [categories])
  const label = useMemo(() => (id: string) => formatPath(index, id, ownNames.get(id)), [index, ownNames])

  const diff = useMemo(() => productCategoriesDiff(label), [label])
  const baseline = useMemo(() => toProductCategoryValues(categories, primaryCategory), [categories, primaryCategory])
  const { form, draft } = useSectionForm({
    id: 'categories',
    title: 'Categories',
    baseline,
    schema: productCategoriesSchema,
    diff,
    routeKey: `product-categories:${productId}`,
    submit: async (v) => {
      const changes = productCategoryChanges(baseline, v)
      // Add new assignments first so a primary swap can target them
      for (const id of changes.add) await addToCategory({ variables: { productId, categoryId: id, isPrimary: false } })
      if (changes.primary) await setPrimary({ variables: { productId, categoryId: changes.primary } })
      // Remove afterwards so we don't temporarily strip the primary if it's being swapped within the same set
      for (const id of changes.remove) await removeFromCategory({ variables: { productId, categoryId: id } })
    },
  })
  const values = useStore(form.store, (s) => s.values)
  const draftAssigned = useMemo(() => new Set(values.assigned), [values.assigned])
  const pending = diff(baseline, values).length

  // The saved assignments in their order, then the staged additions - a staged removal keeps its
  // row, struck through, so it can be undone where it was
  const rows = useMemo(() => {
    const saved = baseline.assigned.map((id) => ({ id, staged: draftAssigned.has(id) ? null : ('remove' as const) }))
    const adds = values.assigned.filter((id) => !baseline.assigned.includes(id)).map((id) => ({ id, staged: 'add' as const }))
    return [...saved, ...adds]
  }, [baseline.assigned, values.assigned, draftAssigned])

  function toggle(id: string) {
    if (!canManage) return
    const next = toggleCategory(form.state.values, id)
    form.setFieldValue('assigned', next.assigned)
    form.setFieldValue('primaryId', next.primaryId)
  }

  function star(id: string) {
    if (!canManage || !draftAssigned.has(id)) return
    form.setFieldValue('primaryId', id)
  }

  const addButton = canManage ? (
    <Button type="button" variant="outline" size="sm" onClick={() => setPickerOpen(true)}>
      <PlusIcon className="size-4" />
      Add categories
    </Button>
  ) : null

  return (
    <div className="grid gap-3">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_CATEGORY_LABELS} />}
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="grid gap-0.5">
          <Label className="text-sm font-medium">Categories</Label>
          <p className="text-xs text-muted-foreground">
            The star marks the primary category, used for the canonical URL and breadcrumbs. Changes apply when you save the page.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="secondary" className="gap-1">
            {draftAssigned.size} assigned
            {pending > 0 ? <span className="text-muted-foreground">· {pending} pending</span> : null}
          </Badge>
          {rows.length > 0 ? addButton : null}
        </div>
      </div>

      {rows.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-md border border-dashed bg-muted/20 px-6 py-8 text-center">
          <p className="text-sm text-muted-foreground">This product isn't in any category yet.</p>
          {addButton}
        </div>
      ) : (
        <ul className="divide-y rounded-md border" aria-label="Assigned categories">
          {rows.map(({ id, staged }) => {
            const path = pathOf(index, id)
            const name = path.at(-1) ?? ownNames.get(id) ?? id
            const parents = path.slice(0, -1)
            const isPrimary = values.primaryId === id
            const removed = staged === 'remove'
            return (
              <li key={id} className="flex items-center gap-2 px-3 py-2 text-sm">
                {removed ? (
                  <span className="size-7 shrink-0" />
                ) : (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className={cn('shrink-0', isPrimary ? 'text-warning' : 'text-muted-foreground/50 hover:text-warning')}
                    onClick={() => star(id)}
                    disabled={!canManage || isPrimary}
                    aria-label={isPrimary ? `${name} is the primary category` : `Make ${name} the primary category`}
                    aria-pressed={isPrimary}
                    title={isPrimary ? 'Primary category' : 'Set as primary'}
                  >
                    <StarIcon className={cn('size-4', isPrimary && 'fill-warning')} />
                  </Button>
                )}
                <span
                  className={cn('min-w-0 flex-1 truncate', removed && 'text-muted-foreground line-through')}
                  title={label(id)}
                >
                  {parents.length ? <span className="text-muted-foreground">{parents.join(PATH_SEPARATOR) + PATH_SEPARATOR}</span> : null}
                  <span className="font-medium">{name}</span>
                </span>
                {isPrimary && !removed ? <span className="shrink-0 text-xs text-muted-foreground">Primary</span> : null}
                {staged === 'add' ? (
                  <span className="shrink-0 text-xs text-success">Will add</span>
                ) : removed ? (
                  <span className="shrink-0 text-xs text-destructive">Will remove</span>
                ) : null}
                {canManage ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    onClick={() => toggle(id)}
                    aria-label={removed ? `Undo removing ${name}` : staged === 'add' ? `Undo adding ${name}` : `Remove ${name}`}
                    title={removed || staged === 'add' ? 'Undo' : 'Remove'}
                  >
                    {removed || staged === 'add' ? <Undo2Icon className="size-4" /> : <Trash2Icon className="size-4" />}
                  </Button>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}

      {canManage ? (
        <CategoryPickerDialog
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          title="Add categories"
          description="Tick every category this product belongs in. Nothing is saved until you save the page."
          takenIds={draftAssigned}
          takenLabel="Assigned"
          onConfirm={(ids) => {
            let next = form.state.values
            for (const id of ids) if (!next.assigned.includes(id)) next = toggleCategory(next, id)
            form.setFieldValue('assigned', next.assigned)
            form.setFieldValue('primaryId', next.primaryId)
          }}
        />
      ) : null}
    </div>
  )
}
