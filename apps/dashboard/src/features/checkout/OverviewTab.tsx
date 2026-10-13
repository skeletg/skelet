import { useState } from 'react'
import { useApolloClient } from '@apollo/client/react'
import { DataTable } from '@/components/data-table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { AdoptCheckoutOptionsDocument } from './Checkout.gql'
import { StatusBadge } from './controls'
import { optionStatus } from './labels'
import { ReadinessList, type GoTo } from './ReadinessList'
import type { CheckoutDraft } from './useCheckoutDraft'
import type { Configuration, Option } from './types'

const GUIDE = [
  { tab: 'delivery', title: 'Delivery', text: 'Delivery to the shopper’s address, store pickup and pickup points, and what each one asks for.' },
  { tab: 'points', title: 'Pickup points', text: 'The lockers, parcel shops and PO boxes a pickup point option delivers to.' },
] as const

export function OverviewTab({
  channelId,
  configuration,
  draft,
  canManage,
  onGo,
  onAdopted,
  hasUnsavedChanges,
}: {
  channelId: string
  configuration: Configuration
  draft: CheckoutDraft
  canManage: boolean
  hasUnsavedChanges: boolean
  onGo: GoTo
  onAdopted: () => Promise<void>
}) {
  const client = useApolloClient()
  const [adopting, setAdopting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // A lifecycle action, not a staged edit: it saves the built-in options as real ones and changes nothing a shopper sees
  async function adopt() {
    setAdopting(true)
    setError(null)
    try {
      await client.mutate({ mutation: AdoptCheckoutOptionsDocument, variables: { channelId } })
      await onAdopted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not start customising checkout')
      setAdopting(false)
    }
  }

  const server = (id: string) => configuration.options.find((o) => o.id === id)
  const failing = configuration.readiness.filter((c) => !c.ok && !c.advisory).length

  return (
    <>
      {!draft.adopted && (
        <DetailPageSection title="Start here">
          <div className="grid gap-3 rounded-md border border-info/40 bg-info-soft p-4">
            <p className="text-sm">
              This channel’s checkout runs on the built-in options:{' '}
              <strong>{configuration.options.map((o) => o.name).join(', ')}</strong>. Shoppers can already check out.
              To rename them, add new ones or offer pickup points, start customising. Nothing changes
              for shoppers until you save.
            </p>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            {hasUnsavedChanges && <p className="text-sm">Save your current checkout changes before starting customisation.</p>}
            <div>
              <Button disabled={!canManage || adopting || hasUnsavedChanges} onClick={adopt}>
                {adopting ? 'Preparing…' : 'Start customising'}
              </Button>
            </div>
          </div>
        </DetailPageSection>
      )}

      <DetailPageSection
        title="Ready to sell?"
        description={
          failing
            ? `${failing} thing${failing === 1 ? '' : 's'} to fix before every shopper can finish checking out.`
            : 'Shoppers can finish checking out on this channel.'
        }
      >
        <ReadinessList checks={configuration.readiness} onGo={onGo} />
      </DetailPageSection>

      <DetailPageSection
        title="Delivery options"
        description="What shoppers choose between at checkout. Select one to edit it."
        trailing={
          <Button variant="outline" size="sm" onClick={() => onGo('delivery')}>
            Manage delivery
          </Button>
        }
      >
        <DataTable<Option>
          compact
          data={draft.options}
          getRowId={(o) => o.id}
          onRowClick={(o) => onGo('delivery', { option: o.id })}
          emptyState="No delivery options yet. Physical products can’t be bought until you add one."
          columns={[
            { accessorKey: 'name', header: 'Option' },
            {
              id: 'kind',
              header: 'Kind',
              accessorFn: (o) => configuration.kinds.find((k) => k.slug === o.kind)?.label ?? o.kind,
            },
            {
              id: 'status',
              header: 'Status',
              cell: ({ row }) => <StatusBadge status={optionStatus(server(row.original.id), row.original.enabled)} />,
            },
          ]}
        />
      </DetailPageSection>

      <DetailPageSection title="How checkout is set up">
        <ul className="grid gap-2 sm:grid-cols-2">
          {GUIDE.map((g) => (
            <li key={g.tab}>
              <button
                type="button"
                className="grid w-full gap-0.5 rounded-md border p-3 text-left transition-colors hover:bg-muted"
                onClick={() => onGo(g.tab)}
              >
                <span className="text-sm font-medium">{g.title}</span>
                <span className="text-sm text-muted-foreground">{g.text}</span>
              </button>
            </li>
          ))}
        </ul>
      </DetailPageSection>
    </>
  )
}
