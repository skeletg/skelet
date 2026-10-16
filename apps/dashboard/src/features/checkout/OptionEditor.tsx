import { Link } from 'react-router-dom'
import { Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { sameJson } from '@/shared/saving/diff'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Block, Choice, Row, StatusBadge } from './controls'
import { MODE_LABELS, optionStatus } from './labels'
import { ReadinessList, type GoTo } from './ReadinessList'
import { carrierChoices } from './labels'
import type { CheckoutDraft } from './useCheckoutDraft'
import { blockMode, type Configuration, type Kind, type Mode, type Option, type Source } from './types'

function sourcesFor(sources: readonly Source[], kind: string) {
  return sources
    .filter((s) => !s.kinds.length || s.kinds.includes(kind))
    .map((s) => ({ value: s.slug, label: s.active ? s.label : `${s.label} (inactive)` }))
}

// Everything about one delivery option, top to bottom in the order a shopper meets it
export function OptionEditor({
  option,
  configuration,
  draft,
  disabled,
  onGo,
}: {
  option: Option
  configuration: Configuration
  draft: CheckoutDraft
  disabled: boolean
  onGo: GoTo
}) {
  const kind = configuration.kinds.find((k) => k.slug === option.kind)
  const server = configuration.options.find((o) => o.id === option.id)
  const saved = draft.baseOptions.find((o) => o.id === option.id)
  const patch = (p: Partial<Option>) => draft.patchOption(option.id, p)
  const status = optionStatus(server, option.enabled)
  const carriers = configuration.carrierKinds.find((c) => c.kind === option.kind)?.carriers ?? []
  const carrierRefs = configuration.carrierKinds.find((c) => c.kind === option.kind)?.carrierRefs ?? []

  return (
    <fieldset disabled={disabled} className="grid min-w-0 gap-8">
      <div className="flex flex-wrap items-center gap-3">
        <h3 className="min-w-0 flex-1 truncate text-lg font-semibold">{option.name || 'Untitled option'}</h3>
        <StatusBadge status={status} />
        <Button variant="outline" size="sm" onClick={() => draft.deleteOption(option.id)}>
          <Trash2Icon className="size-4" /> Remove option
        </Button>
      </div>

      <Block title="Basics">
        <div className="grid gap-3 sm:grid-cols-2">
          <Row label="Name" hint="Shown to shoppers." htmlFor="option-name">
            <Input id="option-name" aria-invalid={!option.name.trim()} value={option.name} onChange={(e) => patch({ name: e.target.value })} />
            {!option.name.trim() && <p role="alert" className="text-xs text-destructive">Name is required.</p>}
          </Row>
          <Row label="Display order" hint="Lower numbers are listed first." htmlFor="option-order">
            <Input id="option-order" type="number" value={option.sortOrder} onChange={(e) => patch({ sortOrder: Number(e.target.value) })} />
          </Row>
        </div>
        <Row label="Description" hint="Optional - a line under the name at checkout." htmlFor="option-description">
          <Input id="option-description" value={option.description} onChange={(e) => patch({ description: e.target.value })} />
        </Row>
        <p className="text-sm text-muted-foreground">
          <strong className="font-medium text-foreground">{kind?.label ?? option.kind}</strong>
          {kind?.description ? ` - ${kind.description}` : ''}
        </p>
      </Block>

      <Block
        title="Offer at checkout"
        description={
          'Stage this with your setup changes. Saving checks that the option is ready before offering it to shoppers.'
        }
      >
        <label className="flex items-center gap-3 text-sm">
          <Switch checked={option.enabled} onCheckedChange={(enabled) => patch({ enabled })} aria-label="Offer at checkout" />
          {option.enabled ? (server?.enabled && server.ready ? 'On' : 'Will be offered after saving') : 'Off'}
        </label>
      </Block>

      {kind && <ShopperProvides kind={kind} option={option} onChange={(blockModes) => patch({ blockModes })} />}

      <Block title="Where it goes">
        {kind?.shipTo.toLowerCase() === 'point' && (
          <Choice
            label="Pickup location source"
            hint={option.kind === 'store_pickup' ? 'Default: active, pickup-enabled inventory locations assigned to this channel.' : 'Default: pickup points listed on the Pickup points tab.'}
            value={option.pointSource}
            noneLabel="Default"
            options={sourcesFor(configuration.pointSources, option.kind)}
            onChange={(pointSource) => patch({ pointSource })}
          />
        )}
        {option.kind === 'parcel_locker' && (option.pointSource === '' || option.pointSource === 'manual-points') && (
          <div>
            <Button variant="outline" size="sm" onClick={() => onGo('points')}>
              Manage pickup points
            </Button>
          </div>
        )}
        {option.kind !== 'store_pickup' && (
          <Choice
            label="Carrier"
            hint="Limit the option to one carrier's methods - two options of the same kind should each name a different carrier, or shoppers see every method twice."
            value={option.carrierSlug}
            noneLabel="Any carrier"
            options={carrierChoices(carrierRefs, option.carrierSlug)}
            onChange={(carrierSlug) => patch({ carrierSlug })}
          />
        )}
        {option.kind !== 'store_pickup' && (
          <p className="text-sm text-muted-foreground">
            {carriers.length ? `Rates come from ${carriers.join(', ')}. ` : 'No shipping method prices this kind yet. '}
            <Link to="/shipping" className="underline underline-offset-4">
              Manage rates in Shipping
            </Link>
          </p>
        )}
      </Block>

      <Block title="Setup checklist" description={server ? (saved && !sameJson(saved, option) ? 'As last saved - save to recheck your changes.' : undefined) : 'Checked after you save.'}>
        {server && <ReadinessList checks={server.readiness} optionId={option.id} onGo={onGo} hideFixFor="delivery" />}
      </Block>
    </fieldset>
  )
}

function ShopperProvides({ kind, option, onChange }: { kind: Kind; option: Option; onChange: (modes: Record<string, Mode>) => void }) {
  return (
    <Block title="What the shopper provides" description="Email and payment are asked on every order.">
      <ul className="divide-y rounded-md border">
        {kind.blocks.map((b) => {
          const mode = blockMode(b, option.blockModes)
          const modes: Mode[] = ['required', 'optional', 'hidden']
          return (
            <li key={b.type} className="flex flex-wrap items-center gap-3 p-3">
              <div className="grid min-w-0 flex-1 gap-0.5">
                <span className="text-sm font-medium">{b.label}</span>
                {b.reason ? <span className="text-xs text-muted-foreground">{b.reason}</span> : null}
              </div>
              {b.floor === 'LOCKED' ? (
                <Badge variant="secondary">Always asked</Badge>
              ) : b.floor === 'NA' ? (
                <Badge variant="outline">Not needed</Badge>
              ) : (
                <div className="w-40">
                  <Choice
                    label={<span className="sr-only">{b.label}</span>}
                    value={mode}
                    options={modes.map((m) => ({ value: m, label: MODE_LABELS[m] }))}
                    onChange={(m) => onChange({ ...option.blockModes, [b.type]: m as Mode })}
                  />
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </Block>
  )
}
