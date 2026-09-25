import { useMemo } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { CustomerGroupsDocument } from '../../customers/CustomerGroups.gql'
import { ProductDocument, SetProductGroupPricesDocument } from '../Catalog.gql'
import { PRODUCT_TIER_LABELS, productTiersDiff, productTiersSchema, toProductTierInput, toProductTierValues } from '../schemas/product'

// Tier (customer-group / quantity-break) pricing - the whole row set stages as one section and saves
// through a replace-all mutation. "All groups" rows apply to every customer; group rows win only for
// members. Sits as a nested block inside the Advanced collapsible

type ExistingGroupPrice = {
  id: string
  customerGroupId?: string | null
  minQty: number
  price: number
}

export function TierPricingSection({
  productId,
  groupPrices,
  currencyCode,
  canManage,
  productVariables,
}: {
  productId: string
  groupPrices: ExistingGroupPrice[]
  currencyCode: string
  canManage: boolean
  productVariables: { id: string }
}) {
  const groupsQuery = useQuery(CustomerGroupsDocument)
  const groups = useMemo(() => groupsQuery.data?.customerGroups ?? [], [groupsQuery.data])
  const [setPrices] = useMutation(SetProductGroupPricesDocument, {
    refetchQueries: [{ query: ProductDocument, variables: productVariables }],
  })

  const baseline = useMemo(() => toProductTierValues(groupPrices), [groupPrices])
  const diff = useMemo(() => productTiersDiff((id) => (id ? (groups.find((g) => g.id === id)?.name ?? 'Group') : 'All groups')), [groups])
  const { form, draft } = useSectionForm({
    id: 'tier-pricing',
    title: 'Tier pricing',
    baseline,
    schema: productTiersSchema,
    diff,
    routeKey: `product-tiers:${productId}`,
    submit: async (v) => {
      await setPrices({ variables: { productId, prices: toProductTierInput(v) } })
    },
  })
  const groupOptions = groups.map((g) => ({ value: g.id, label: g.name }))
  const cols = 'grid grid-cols-[minmax(9rem,13rem)_5.5rem_8.5rem_minmax(0,1fr)_2.25rem] items-start gap-3'

  return (
    <div className="grid gap-3 rounded-lg border border-dashed p-4">
      <div>
        <div className="text-sm font-medium">Tier pricing</div>
        <p className="text-xs text-muted-foreground">
          Quantity-break prices per customer group. The lowest applicable tier beats the regular price in the cart.
        </p>
      </div>
      {draft.restoredAt !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_TIER_LABELS} />}
      <fieldset disabled={!canManage} className="grid gap-2">
        <form.Field name="tiers" mode="array">
          {(tiers) => (
            <>
              {tiers.state.value.length === 0 ? (
                <div className="rounded-lg border border-dashed p-6 text-center">
                  <p className="text-sm text-muted-foreground">No tier prices yet.</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Offer a lower per-unit price once the cart reaches a quantity - optionally scoped to a customer group.
                  </p>
                </div>
              ) : (
                <>
                  <div className={`${cols} px-3 text-xs font-medium text-muted-foreground`}>
                    <span>Customer group</span>
                    <span>Min qty</span>
                    <span>Price ({currencyCode})</span>
                    <span aria-hidden />
                    <span aria-hidden />
                  </div>
                  {tiers.state.value.map((row, i) => {
                    const q = Number(row.minQty)
                    const pr = Number(row.price)
                    const preview = q > 0 && row.price.trim() !== '' && Number.isFinite(pr) ? `${q}+ → ${pr.toFixed(2)} each` : ''
                    return (
                      <div key={i} className={`${cols} rounded-lg border bg-muted/20 px-3 py-2`}>
                        <form.AppField name={`tiers[${i}].customerGroupId`}>
                          {(f) => <f.SelectField id={`tier-${i}-group`} options={groupOptions} noneLabel="All groups" disabled={!canManage} />}
                        </form.AppField>
                        <form.AppField name={`tiers[${i}].minQty`}>
                          {(f) => <f.TextField aria-label={`Min qty, tier ${i + 1}`} type="number" min="0" step="any" inputMode="decimal" className="[&_input]:h-9" />}
                        </form.AppField>
                        <form.AppField name={`tiers[${i}].price`}>
                          {(f) => <f.TextField aria-label={`Price, tier ${i + 1}`} type="number" min="0" step="0.01" inputMode="decimal" className="[&_input]:h-9" />}
                        </form.AppField>
                        <span className="truncate pt-2.5 text-xs tabular-nums text-muted-foreground">{preview}</span>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="size-9 text-muted-foreground hover:text-destructive"
                          onClick={() => tiers.removeValue(i)}
                          aria-label={`Remove tier ${i + 1}`}
                        >
                          <Trash2Icon className="size-4" />
                        </Button>
                      </div>
                    )
                  })}
                </>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                onClick={() => tiers.pushValue({ customerGroupId: '', minQty: tiers.state.value.length === 0 ? '1' : '', price: '' })}
              >
                <PlusIcon className="size-4" /> Add tier
              </Button>
            </>
          )}
        </form.Field>
      </fieldset>
    </div>
  )
}
