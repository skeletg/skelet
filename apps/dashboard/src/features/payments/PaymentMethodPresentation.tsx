import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope } from '@/lib/config-scope'
import { changed } from '@/shared/saving/diff'
import { useSaveSection } from '@/shared/saving/saveRegistry'
import { PaymentMethodsDocument, UpdatePaymentMethodPresentationDocument } from './Payments.gql'

export function PaymentMethodPresentation({ disabled }: { disabled?: boolean }) {
  const { effectiveChannelId } = useConfigScope()
  const permissions = usePermissions()
  const canManage = !!effectiveChannelId && permissions.canAccessChannelScope('payments.manage', effectiveChannelId)
  const { data, error } = useQuery(PaymentMethodsDocument, { variables: { channelId: effectiveChannelId ?? '' }, skip: !effectiveChannelId })
  const [update] = useMutation(UpdatePaymentMethodPresentationDocument, { refetchQueries: [{ query: PaymentMethodsDocument, variables: { channelId: effectiveChannelId ?? '' } }], awaitRefetchQueries: true })
  const [drafts, setDrafts] = useState<Record<string, { displayName: string; sortOrder: number }>>({})
  const methods = data?.paymentMethods ?? []
  const pending = methods.flatMap((method) => {
    const after = drafts[method.providerSlug]
    return after && (after.displayName !== method.displayName || after.sortOrder !== method.sortOrder) ? [{ method, after }] : []
  })
  const diff = pending.flatMap(({ method, after }) => [changed(`${method.displayName} · Checkout label`, method.displayName, after.displayName), changed(`${method.displayName} · Display order`, method.sortOrder, after.sortOrder)].filter((d) => d !== null))
  useSaveSection({ id: 'payment-presentation', title: 'Payment labels and order', diff, save: async () => {
    for (const { after } of pending) {
      if (!after.displayName.trim() || !Number.isInteger(after.sortOrder)) throw new Error('Every payment method needs a checkout label and a whole-number display order.')
    }
    for (const { method, after } of pending) await update({ variables: { channelId: effectiveChannelId!, providerSlug: method.providerSlug, ...after } })
    setDrafts({})
  } })
  return <DetailPageSection title="Labels and display order" description="Names shoppers see at checkout. Lower display-order numbers appear first.">
    {error ? <p role="alert" className="text-sm text-destructive">{error.message}</p> : <div className="divide-y rounded-md border">{methods.map((method) => {
      const value = drafts[method.providerSlug] ?? method
      const patch = (next: Partial<{ displayName: string; sortOrder: number }>) => setDrafts((prev) => ({ ...prev, [method.providerSlug]: { displayName: value.displayName, sortOrder: value.sortOrder, ...next } }))
      return <fieldset disabled={disabled || !canManage} key={method.providerSlug} className="grid gap-3 p-4 sm:grid-cols-[1fr_10rem]">
        <div className="grid gap-1"><Label htmlFor={`payment-label-${method.providerSlug}`}>{method.providerSlug} · Checkout label</Label><Input id={`payment-label-${method.providerSlug}`} value={value.displayName} onChange={(e) => patch({ displayName: e.target.value })} aria-invalid={!value.displayName.trim()} />{!value.displayName.trim() && <p role="alert" className="text-xs text-destructive">Checkout label is required.</p>}</div>
        <div className="grid gap-1"><Label htmlFor={`payment-order-${method.providerSlug}`}>Display order</Label><Input id={`payment-order-${method.providerSlug}`} type="number" step="1" value={value.sortOrder} onChange={(e) => patch({ sortOrder: Number(e.target.value) })} /></div>
      </fieldset>
    })}</div>}
  </DetailPageSection>
}
