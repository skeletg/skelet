import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CustomerSearch } from '@/features/carts/pickers'
import { CustomerGroupsDocument } from '@/features/customers/CustomerGroups.gql'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { formatCents } from '@/lib/format'
import { AutocompletePicker, type Option } from '@/features/promotions/widgets/AutocompletePicker'
import { PluginSections } from '@/plugins/detail-sections'
import { MasqueradeButton } from './MasqueradeButton'

import {
  CompanyDetailDocument,
  CompanyInvoicesDocument,
  CompanyPricePickerProductsDocument,
  DeleteCompanyPriceDocument,
  ReleaseCompanyCreditDocument,
  RemoveCompanyMemberDocument,
  SetCompanyCustomerGroupDocument,
  SetCompanyMemberDocument,
  SetCompanyPriceDocument,
  SettleB2bInvoiceDocument,
  UpdateCompanyDocument,
} from './B2B.gql'

function toCents(v: string): number {
  return Math.round(Number.parseFloat(v || '0') * 100)
}

export function CompanyDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const groupOptions = useQuery(CustomerGroupsDocument).data?.customerGroups ?? []
  const { data, loading, error, refetch } = useQuery(CompanyDetailDocument, {
    variables: { id },
    skip: !id,
  })

  const [update] = useMutation(UpdateCompanyDocument)
  const [setGroup] = useMutation(SetCompanyCustomerGroupDocument)
  const [setMember] = useMutation(SetCompanyMemberDocument)
  const [removeMember] = useMutation(RemoveCompanyMemberDocument)
  const [release] = useMutation(ReleaseCompanyCreditDocument)

  const [groupSlug, setGroupSlug] = useState<string | null>(null)
  const [memberId, setMemberId] = useState('')
  const [memberRole, setMemberRole] = useState('buyer')
  const [memberLimit, setMemberLimit] = useState('0')
  const [releaseAmount, setReleaseAmount] = useState('')
  const [releaseReason, setReleaseReason] = useState('')

  const co = data?.company

  const run = async (fn: () => Promise<unknown>, ok?: string) => {
    try {
      await fn()
      await refetch()
      if (ok) toast.success(ok)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Operation failed')
    }
  }

  if (loading && !co) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading company…</p>
  }
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load company"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!co) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Company not found.</p>
  }

  const available = co.creditLimitCents - co.creditUsedCents

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/b2b/companies"
        backLabel="Companies"
        title={co.name}
        meta={`Net ${co.paymentTermsDays} · ${formatCents(co.creditUsedCents, 'USD')} used of ${formatCents(co.creditLimitCents, 'USD')} (${formatCents(available, 'USD')} available)`}
        actions={
          <>
            <Badge variant={co.status === 'active' ? 'default' : 'destructive'}>{co.status}</Badge>
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                void run(
                  () =>
                    update({
                      variables: {
                        id,
                        name: co.name,
                        status: co.status === 'active' ? 'suspended' : 'active',
                        creditLimitCents: co.creditLimitCents,
                        paymentTermsDays: co.paymentTermsDays,
                      },
                    }),
                  co.status === 'active' ? 'Company suspended' : 'Company reactivated',
                )
              }
            >
              {co.status === 'active' ? 'Suspend' : 'Reactivate'}
            </Button>
          </>
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Price group"
          description="Customer-group slug; members are moved into it (group + tier prices apply). Empty clears back to retail."
        >
          <div className="flex max-w-md gap-2">
            <Select
              value={(groupSlug ?? co.defaultCustomerGroupSlug) || '__none__'}
              onValueChange={(v) => setGroupSlug(v === '__none__' ? '' : v)}
            >
              <SelectTrigger className="flex-1">
                <SelectValue placeholder="Pick a customer group" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">- Clear back to retail -</SelectItem>
                {groupOptions.map((g) => (
                  <SelectItem key={g.id} value={g.slug}>
                    {g.name || g.slug}
                    <span className="ml-1 font-mono text-xs text-muted-foreground">{g.slug}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              onClick={() =>
                void run(
                  () =>
                    setGroup({
                      variables: { companyId: id, groupSlug: (groupSlug ?? co.defaultCustomerGroupSlug).trim() },
                    }),
                  'Price group saved',
                )
              }
            >
              Save
            </Button>
          </div>
        </DetailPageSection>

        <ContractPricesSection companyId={id} prices={co.prices} onChanged={() => void refetch()} />

        <DetailPageSection title="Members" description="Paste the customer id from the customer detail page URL.">
          {(data?.companyMembers ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No members yet.</p>
          ) : (
            <ul className="divide-y rounded-md border">
              {(data?.companyMembers ?? []).map((m) => (
                <li key={m.customerId} className="flex items-center justify-between px-3 py-2 text-sm">
                  <span>
                    <Link to={`/customers/${m.customerId}`} className="underline-offset-2 hover:underline">
                      {m.email}
                    </Link>{' '}
                    <Badge variant="secondary">{m.role}</Badge>
                    {m.orderLimitCents > 0 && (
                      <span className="ml-2 text-xs text-muted-foreground">
                        max {formatCents(m.orderLimitCents, 'USD')}/order
                      </span>
                    )}
                  </span>
                  <span className="flex items-center gap-1">
                    <MasqueradeButton customerId={m.customerId} variant="ghost" label="Shop as" />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        void run(() => removeMember({ variables: { customerId: m.customerId } }), 'Member removed')
                      }
                    >
                      Remove
                    </Button>
                  </span>
                </li>
              ))}
            </ul>
          )}
          <div className="grid max-w-2xl grid-cols-[2fr_1fr_1fr_auto] gap-2">
            <CustomerSearch onPick={(c) => setMemberId(c.id)} />
            <Input placeholder="buyer|admin" value={memberRole} onChange={(e) => setMemberRole(e.target.value)} />
            <Input placeholder="Limit $ (0=∞)" type="number" step="0.01" value={memberLimit} onChange={(e) => setMemberLimit(e.target.value)} />
            <Button
              variant="outline"
              disabled={!memberId.trim()}
              onClick={() =>
                void run(
                  () =>
                    setMember({
                      variables: {
                        companyId: id,
                        customerId: memberId.trim(),
                        role: memberRole.trim() || 'buyer',
                        orderLimitCents: toCents(memberLimit),
                      },
                    }),
                  'Member added',
                ).then(() => setMemberId(''))
              }
            >
              Add
            </Button>
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Credit"
          description="On-account (purchase order) exposure. Releasing credit lowers the used amount - settling an invoice below does it automatically."
        >
          <div className="grid max-w-2xl grid-cols-[1fr_2fr_auto] gap-2">
            <Input placeholder="Amount $" type="number" step="0.01" min="0.01" value={releaseAmount} onChange={(e) => setReleaseAmount(e.target.value)} />
            <Input placeholder="Reason (invoice #…)" value={releaseReason} onChange={(e) => setReleaseReason(e.target.value)} />
            <Button
              variant="outline"
              disabled={!releaseAmount}
              onClick={() =>
                void run(
                  () =>
                    release({
                      variables: {
                        companyId: id,
                        amountCents: toCents(releaseAmount),
                        reason: releaseReason.trim() || null,
                      },
                    }),
                  'Credit released',
                ).then(() => {
                  setReleaseAmount('')
                  setReleaseReason('')
                })
              }
            >
              Release
            </Button>
          </div>
          {(data?.companyCreditEntries ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No credit activity yet.</p>
          ) : (
            <ul className="divide-y rounded-md border text-sm">
              {(data?.companyCreditEntries ?? []).map((e) => (
                <li key={e.id} className="grid grid-cols-[auto_1fr_auto] items-center gap-3 px-3 py-1.5">
                  <span className="whitespace-nowrap text-muted-foreground">
                    {new Date(e.createdAt as string).toLocaleString()}
                  </span>
                  <span>{e.reason}</span>
                  <span className={`text-right tabular-nums ${e.amountCents < 0 ? 'text-emerald-600 dark:text-emerald-400' : ''}`}>
                    {e.amountCents > 0 ? '+' : ''}
                    {formatCents(e.amountCents, 'USD')}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </DetailPageSection>

        <InvoicesSection companyId={id} onChanged={() => void refetch()} />

        <PluginSections target="company" entity={co} mode="edit" />
      </DetailPageBody>
    </DetailPageLayout>
  )
}

type Price = { id: string; productId: string; productSku: string; priceCents: number }

// Contract prices - a negotiated flat unit price per product that replaces
// list/tier pricing for this company's members at cart time
function ContractPricesSection({
  companyId,
  prices,
  onChanged,
}: {
  companyId: string
  prices: Price[]
  onChanged: () => void
}) {
  const [setPrice] = useMutation(SetCompanyPriceDocument)
  const [deletePrice] = useMutation(DeleteCompanyPriceDocument)
  const { data: pickerData, loading: pickerLoading } = useQuery(CompanyPricePickerProductsDocument)

  const [productId, setProductId] = useState('')
  const [newPrice, setNewPrice] = useState('')
  const [edits, setEdits] = useState<Record<string, string>>({})

  const options = useMemo<Option[]>(
    () =>
      (pickerData?.products ?? [])
        .filter((p) => !prices.some((row) => row.productId === p.id))
        .map((p) => ({ value: p.id, label: p.name || p.sku, hint: p.sku })),
    [pickerData, prices],
  )

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      onChanged()
      toast.success(ok)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Operation failed')
    }
  }

  return (
    <DetailPageSection
      title="Contract prices"
      description="Negotiated flat unit prices - they replace list/tier pricing for members of this company at cart time."
    >
      {prices.length === 0 ? (
        <p className="text-sm text-muted-foreground">No contract prices yet.</p>
      ) : (
        <ul className="divide-y rounded-md border text-sm">
          {prices.map((row) => {
            const draft = edits[row.id]
            const dirty = draft != null && toCents(draft) !== row.priceCents
            return (
              <li key={row.id} className="grid grid-cols-[1fr_10rem_auto_auto] items-center gap-2 px-3 py-1.5">
                <span className="font-mono">{row.productSku}</span>
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  className="h-8 text-right tabular-nums"
                  value={draft ?? (row.priceCents / 100).toFixed(2)}
                  onChange={(e) => setEdits((s) => ({ ...s, [row.id]: e.target.value }))}
                />
                <Button
                  variant="outline"
                  size="sm"
                  className={dirty ? '' : 'invisible'}
                  onClick={() =>
                    void run(
                      () =>
                        setPrice({
                          variables: { companyId, productId: row.productId, priceCents: toCents(draft ?? '') },
                        }),
                      'Price updated',
                    ).then(() =>
                      setEdits((s) => {
                        const next = { ...s }
                        delete next[row.id]
                        return next
                      }),
                    )
                  }
                >
                  Save
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7 text-muted-foreground hover:text-destructive"
                  onClick={() =>
                    void run(() => deletePrice({ variables: { companyId, priceId: row.id } }), 'Price removed')
                  }
                >
                  <Trash2Icon className="size-3.5" />
                </Button>
              </li>
            )
          })}
        </ul>
      )}
      <div className="grid max-w-2xl grid-cols-[2fr_10rem_auto] items-end gap-2">
        <div className="grid gap-1.5">
          <Label>Product</Label>
          <AutocompletePicker
            options={options}
            value={productId}
            onChange={(v) => setProductId((v as string) ?? '')}
            placeholder="Select product…"
            emptyMessage="No products."
            loading={pickerLoading}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="cp-price">Unit price $</Label>
          <Input id="cp-price" type="number" step="0.01" min="0" value={newPrice} onChange={(e) => setNewPrice(e.target.value)} />
        </div>
        <Button
          variant="outline"
          disabled={!productId || newPrice === ''}
          onClick={() =>
            void run(
              () => setPrice({ variables: { companyId, productId, priceCents: toCents(newPrice) } }),
              'Contract price added',
            ).then(() => {
              setProductId('')
              setNewPrice('')
            })
          }
        >
          Add
        </Button>
      </div>
    </DetailPageSection>
  )
}

// On-account invoices - one per purchase-order order - Settling (paid/void)
// releases the reserved company credit in the same transaction
function InvoicesSection({ companyId, onChanged }: { companyId: string; onChanged: () => void }) {
  const { data, loading, refetch } = useQuery(CompanyInvoicesDocument, {
    variables: { companyId, limit: 100 },
  })
  const [settle, { loading: settling }] = useMutation(SettleB2bInvoiceDocument)

  const invoices = data?.companyInvoices ?? []

  const doSettle = async (invoiceId: string, status: 'paid' | 'void') => {
    try {
      await settle({ variables: { id: invoiceId, status } })
      await refetch()
      onChanged()
      toast.success(status === 'paid' ? 'Invoice marked paid - credit released' : 'Invoice voided - credit released')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Settle failed')
    }
  }

  return (
    <DetailPageSection
      title="Invoices"
      description="Created automatically when an on-account order is placed; due date follows the company's payment terms."
    >
      {loading && invoices.length === 0 ? (
        <p className="text-sm text-muted-foreground">Loading invoices…</p>
      ) : invoices.length === 0 ? (
        <p className="text-sm text-muted-foreground">No invoices yet.</p>
      ) : (
        <ul className="divide-y rounded-md border text-sm">
          {invoices.map((inv) => (
            <li key={inv.id} className="grid grid-cols-[8rem_8rem_1fr_auto_auto] items-center gap-3 px-3 py-2">
              <span className="font-mono">{inv.number}</span>
              <Link to={`/orders/${inv.orderId}`} className="font-mono underline-offset-2 hover:underline">
                {inv.orderNumber}
              </Link>
              <span className="text-muted-foreground">
                {inv.overdue ? (
                  <Badge variant="destructive">overdue</Badge>
                ) : (
                  <Badge variant={inv.status === 'paid' ? 'default' : inv.status === 'void' ? 'outline' : 'secondary'}>
                    {inv.status}
                  </Badge>
                )}{' '}
                {inv.status === 'paid' && inv.paidAt
                  ? `paid ${new Date(inv.paidAt as string).toLocaleDateString()}`
                  : inv.dueAt
                    ? `due ${new Date(inv.dueAt as string).toLocaleDateString()}`
                    : 'no due date'}
              </span>
              <span className="text-right tabular-nums">{formatCents(inv.amountCents, 'USD')}</span>
              {inv.status === 'open' ? (
                <span className="flex gap-1">
                  <Button variant="outline" size="sm" disabled={settling} onClick={() => void doSettle(inv.id, 'paid')}>
                    Mark paid
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={settling}
                    className="text-muted-foreground hover:text-destructive"
                    onClick={() => void doSettle(inv.id, 'void')}
                  >
                    Void
                  </Button>
                </span>
              ) : (
                <span />
              )}
            </li>
          ))}
        </ul>
      )}
    </DetailPageSection>
  )
}
