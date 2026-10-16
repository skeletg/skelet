import { useId } from 'react'
import { PlusIcon, TrashIcon } from 'lucide-react'
import { DataTable } from '@/components/data-table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Empty } from '@/components/layout/panels'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Row } from './controls'
import { CountryPicker } from '@/features/promotions/widgets/CountryPicker'
import type { CheckoutDraft } from './useCheckoutDraft'
import type { Point } from './types'

// The pickup points (lockers, parcel shops, PO boxes) a pickup point option offers when its points come from this list - the whole list saves at once
export function PointsTab({
  draft,
  selectedKey,
  disabled,
  onSelect,
}: {
  draft: CheckoutDraft
  selectedKey: string | null
  disabled: boolean
  onSelect: (key: string | null) => void
}) {
  const selected = draft.points.find((p) => p.key === selectedKey) ?? draft.points[0]

  return (
    <DetailPageSection
      title="Pickup points"
      description="Lockers, parcel shops and PO boxes a pickup point option can deliver to. Shoppers pick one at checkout, and its address becomes the shipping address. A carrier's own network is connected as a point source on the option instead."
    >
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(16rem,20rem)_1fr]">
        <div className="grid gap-3">
          <DataTable<Point>
            compact
            data={draft.points}
            getRowId={(p) => p.key}
            onRowClick={(p) => onSelect(p.key)}
            rowIsActive={(p) => p.key === selected?.key}
            emptyState="No pickup points yet."
            columns={[
              {
                accessorKey: 'name',
                header: 'Pickup point',
                cell: ({ row }) => (
                  <div className="grid">
                    <span className="font-medium">{row.original.name || 'New pickup point'}</span>
                    <span className="text-xs text-muted-foreground">
                      {[row.original.city, row.original.countryCode].filter(Boolean).join(', ')}
                    </span>
                  </div>
                ),
              },
              {
                id: 'status',
                header: 'Status',
                cell: ({ row }) =>
                  row.original.isActive ? <Badge variant="success">Active</Badge> : <Badge variant="secondary">Off</Badge>,
              },
            ]}
          />
          <Button variant="outline" disabled={disabled} onClick={() => onSelect(draft.addPoint())}>
            <PlusIcon className="size-4" /> Add pickup point
          </Button>
        </div>
        <div className="min-w-0 rounded-md border p-4">
          {selected ? (
            <PointEditor key={selected.key} point={selected} draft={draft} disabled={disabled} onRemoved={() => onSelect(null)} />
          ) : (
            <Empty>Add a locker, parcel shop or PO box shoppers can choose at checkout.</Empty>
          )}
        </div>
      </div>
    </DetailPageSection>
  )
}

function PointEditor({
  point,
  draft,
  disabled,
  onRemoved,
}: {
  point: Point
  draft: CheckoutDraft
  disabled: boolean
  onRemoved: () => void
}) {
  const id = useId()
  const patch = (p: Partial<Point>) => draft.patchPoint(point.key, p)
  const text = (key: keyof Point, label: string, hint?: string) => (
    <Row label={label} hint={hint} htmlFor={`${id}-${key}`}>
      <Input
        id={`${id}-${key}`}
        value={point[key] as string}
        disabled={disabled}
        aria-invalid={['name', 'street1', 'city', 'postalCode'].includes(key) && !(point[key] as string).trim()}
        onChange={(e) => patch({ [key]: e.target.value } as Partial<Point>)}
      />
      {['name', 'street1', 'city', 'postalCode'].includes(key) && !(point[key] as string).trim() && (
        <p role="alert" className="text-xs text-destructive">{label} is required.</p>
      )}
    </Row>
  )
  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Switch
            id={`${id}-active`}
            checked={point.isActive}
            disabled={disabled}
            onCheckedChange={(isActive) => patch({ isActive })}
          />
          <label htmlFor={`${id}-active`} className="text-sm">
            Offered at checkout
          </label>
        </div>
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled}
          onClick={() => {
            draft.removePoint(point.key)
            onRemoved()
          }}
        >
          <TrashIcon className="size-4" /> Remove
        </Button>
      </div>
      {text('name', 'Name', 'What the shopper sees, such as "Main post office" or "Parcel locker, Central station".')}
      <div className="grid gap-4 sm:grid-cols-2">
        {text('street1', 'Street')}
        {text('street2', 'Street, line 2')}
        {text('postalCode', 'Postcode')}
        {text('city', 'City')}
        {text('region', 'Region')}
        <fieldset disabled={disabled} className="grid gap-1.5">
          <legend className="text-sm font-medium">Country</legend>
          <CountryPicker schema={{ type: 'string' }} value={point.countryCode} onChange={(value) => patch({ countryCode: String(value) })} />
          {!point.countryCode && <p role="alert" className="text-xs text-destructive">Country is required.</p>}
        </fieldset>
      </div>
      {text('hours', 'Opening hours', 'Optional, shown under the pickup point at checkout.')}
      <Row label="Instructions" hint="Optional, such as how to collect from the box." htmlFor={`${id}-instructions`}>
        <Textarea
          id={`${id}-instructions`}
          value={point.instructions}
          disabled={disabled}
          onChange={(e) => patch({ instructions: e.target.value })}
        />
      </Row>
    </div>
  )
}
