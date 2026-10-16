import { useState } from 'react'
import { PlusIcon } from 'lucide-react'
import { DataTable } from '@/components/data-table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Empty } from '@/components/layout/panels'
import { Button } from '@/components/ui/button'
import { StatusBadge } from './controls'
import { optionStatus } from './labels'
import { OptionEditor } from './OptionEditor'
import type { GoTo } from './ReadinessList'
import type { CheckoutDraft } from './useCheckoutDraft'
import type { Configuration, Option } from './types'

export function DeliveryTab({
  configuration,
  draft,
  selectedId,
  disabled,
  onSelect,
  onGo,
}: {
  configuration: Configuration
  draft: CheckoutDraft
  selectedId: string | null
  disabled: boolean
  onSelect: (id: string | null) => void
  onGo: GoTo
}) {
  const [choosingKind, setChoosingKind] = useState(false)
  const sorted = [...draft.options].sort((a, b) => a.sortOrder - b.sortOrder)
  const selected = draft.options.find((o) => o.id === selectedId) ?? sorted[0]

  function add(kind: string) {
    setChoosingKind(false)
    onSelect(draft.addOption(kind))
  }

  return (
    <DetailPageSection
      title="Delivery options"
      description="The ways a shopper can receive a physical order. Digital-only orders skip delivery."
    >
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(16rem,20rem)_1fr]">
        <div className="grid gap-3">
          <DataTable<Option>
            compact
            data={sorted}
            getRowId={(o) => o.id}
            onRowClick={(o) => onSelect(o.id)}
            rowIsActive={(o) => o.id === selected?.id}
            emptyState="No delivery options yet."
            columns={[
              {
                accessorKey: 'name',
                header: 'Option',
                cell: ({ row }) => (
                  <div className="grid">
                    <span className="font-medium">{row.original.name || 'Untitled option'}</span>
                    <span className="text-xs text-muted-foreground">
                      {configuration.kinds.find((k) => k.slug === row.original.kind)?.label ?? row.original.kind}
                    </span>
                  </div>
                ),
              },
              {
                id: 'status',
                header: 'Status',
                cell: ({ row }) => (
                  <StatusBadge status={optionStatus(configuration.options.find((o) => o.id === row.original.id), row.original.enabled)} />
                ),
              },
            ]}
          />
          {choosingKind ? (
            <div className="grid gap-2 rounded-md border p-3">
              <p className="text-sm font-medium">What kind of delivery?</p>
              {configuration.kinds.map((k) => (
                <button
                  key={k.slug}
                  type="button"
                  className="grid gap-0.5 rounded-md border p-3 text-left transition-colors hover:bg-muted"
                  onClick={() => add(k.slug)}
                >
                  <span className="text-sm font-medium">{k.label}</span>
                  <span className="text-xs text-muted-foreground">{k.description}</span>
                </button>
              ))}
              <Button variant="ghost" size="sm" onClick={() => setChoosingKind(false)}>
                Cancel
              </Button>
            </div>
          ) : (
            <Button variant="outline" disabled={disabled} onClick={() => setChoosingKind(true)}>
              <PlusIcon className="size-4" /> Add delivery option
            </Button>
          )}
        </div>
        <div className="min-w-0 rounded-md border p-4">
          {selected ? (
            <OptionEditor
              key={selected.id}
              option={selected}
              configuration={configuration}
              draft={draft}
              disabled={disabled}
              onGo={onGo}
            />
          ) : (
            <Empty>Add a delivery option to start.</Empty>
          )}
        </div>
      </div>
    </DetailPageSection>
  )
}
