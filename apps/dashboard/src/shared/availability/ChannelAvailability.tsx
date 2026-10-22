import { useId, useMemo, useState } from 'react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { changed } from '@/shared/saving/diff'
import { useSaveSection } from '@/shared/saving/saveRegistry'

// Per-channel enablement + country rules for things installed platform-wide (payment providers, shipping carriers) - they live in
// their own tables rather than as config fields, so this block stages them itself and applies through the page-header Save
// The caller keys it by channel, so switching channels starts from a clean draft

export type CountryMode = 'all' | 'specific'

export type AvailabilityItem = {
  slug: string
  name: string
  enabled: boolean
  countryMode: string
  allowedCountries?: string[] | null
  badges?: string[]
  unavailableReason?: string
  // A shipping carrier's weekend rule - the toggle shows only for items that set it, so payment providers never see it
  deliversOnWeekends?: boolean
}

type Draft = { enabled: boolean; mode: CountryMode; countries: string; weekends?: boolean }

export type AvailabilityChange = {
  slug: string
  enabled?: boolean
  countries?: { mode: CountryMode; countries: string[] }
  deliversOnWeekends?: boolean
}

function fromItem(item: AvailabilityItem): Draft {
  return {
    enabled: item.enabled,
    mode: item.countryMode === 'specific' ? 'specific' : 'all',
    countries: (item.allowedCountries ?? []).join(', '),
    weekends: item.deliversOnWeekends,
  }
}

function parseCountries(text: string): string[] {
  return text
    .split(',')
    .map((c) => c.trim().toUpperCase())
    .filter(Boolean)
}

function describeCountries(d: Draft): string {
  return d.mode === 'specific' ? parseCountries(d.countries).join(', ') || 'No countries' : 'All countries'
}

export function ChannelAvailability({
  sectionId,
  title,
  description,
  countriesLabel,
  items,
  loading,
  error,
  isChannel,
  emptyText,
  disabled,
  onSave,
}: {
  sectionId: string
  title: string
  description: string
  // "Accepted in", "Ships to"
  countriesLabel: string
  items: AvailabilityItem[]
  loading: boolean
  error?: string
  isChannel: boolean
  emptyText: string
  disabled?: boolean
  // Applies one item's staged changes - only the halves that changed are set
  onSave: (change: AvailabilityChange) => Promise<unknown>
}) {
  const fid = useId()
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})

  const pending = useMemo(() => {
    const out: { item: AvailabilityItem; before: Draft; after: Draft }[] = []
    for (const item of items) {
      const after = drafts[item.slug]
      if (!after) continue
      const before = fromItem(item)
      if (after.enabled !== before.enabled || describeCountries(after) !== describeCountries(before) || after.weekends !== before.weekends) {
        out.push({ item, before, after })
      }
    }
    return out
  }, [items, drafts])

  const diff = pending.flatMap(({ item, before, after }) =>
    [
      changed(`${item.name} · Enabled`, before.enabled, after.enabled),
      changed(`${item.name} · ${countriesLabel}`, describeCountries(before), describeCountries(after)),
      before.weekends === undefined ? null : changed(`${item.name} · Delivers on weekends`, before.weekends, after.weekends),
    ].filter((d) => d !== null),
  )

  useSaveSection({
    id: sectionId,
    title,
    diff,
    save: async () => {
      for (const { item, before, after } of pending) {
        const countriesChanged = describeCountries(after) !== describeCountries(before)
        await onSave({
          slug: item.slug,
          ...(after.enabled !== before.enabled ? { enabled: after.enabled } : {}),
          ...(countriesChanged
            ? { countries: { mode: after.mode, countries: after.mode === 'specific' ? parseCountries(after.countries) : [] } }
            : {}),
          ...(after.weekends !== before.weekends ? { deliversOnWeekends: after.weekends } : {}),
        })
      }
      setDrafts({})
    },
  })

  function edit(item: AvailabilityItem, patch: Partial<Draft>) {
    setDrafts((prev) => ({ ...prev, [item.slug]: { ...(prev[item.slug] ?? fromItem(item)), ...patch } }))
  }

  return (
    <DetailPageSection title={title} description={description}>
      {!isChannel ? (
        <p className="text-sm text-muted-foreground">Availability is per channel. Switch the header scope to a channel to change it.</p>
      ) : loading && items.length === 0 ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : error ? (
        <Alert variant="destructive">
          <AlertTitle>Failed to load</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{emptyText}</p>
      ) : (
        <div className="max-w-3xl divide-y rounded-md border">
          {items.map((item) => {
            const d = drafts[item.slug] ?? fromItem(item)
            return (
              <div key={item.slug} className="grid gap-3 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="flex items-center gap-1.5 text-sm font-medium">
                      {item.name}
                      {item.badges?.map((b) => (
                        <Badge key={b} variant="outline" className="text-[10px] uppercase">
                          {b}
                        </Badge>
                      ))}
                    </p>
                    <p className="font-mono text-[10px] text-muted-foreground">{item.slug}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Label htmlFor={`${sectionId}-enabled-${item.slug}`} className="text-xs text-muted-foreground">
                      {d.enabled ? 'Enabled' : 'Disabled'}
                    </Label>
                    <Switch
                      id={`${sectionId}-enabled-${item.slug}`}
                      checked={d.enabled}
                      disabled={disabled || (!!item.unavailableReason && !item.enabled)}
                      onCheckedChange={(enabled) => edit(item, { enabled })}
                    />
                  </div>
                </div>
                {item.unavailableReason && <p className="text-sm text-warning">{item.unavailableReason}</p>}
                <div className="grid gap-3 sm:grid-cols-[200px_1fr]">
                  <div className="grid gap-1">
                    <Label htmlFor={`${fid}-countries-${item.slug}`} className="text-xs">{countriesLabel}</Label>
                    <Select value={d.mode} disabled={disabled} onValueChange={(v) => edit(item, { mode: v as CountryMode })}>
                      <SelectTrigger id={`${fid}-countries-${item.slug}`} className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">All countries</SelectItem>
                        <SelectItem value="specific">Specific countries</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  {d.mode === 'specific' && (
                    <div className="grid gap-1">
                      <Label htmlFor={`${sectionId}-countries-${item.slug}`} className="text-xs">
                        Countries (ISO-2, comma-separated)
                      </Label>
                      <Input
                        id={`${sectionId}-countries-${item.slug}`}
                        value={d.countries}
                        disabled={disabled || (!!item.unavailableReason && !item.enabled)}
                        onChange={(e) => edit(item, { countries: e.target.value })}
                        placeholder="US, CA"
                      />
                    </div>
                  )}
                </div>
                {d.weekends !== undefined && (
                  <div className="flex items-start gap-2">
                    <Switch
                      id={`${sectionId}-weekends-${item.slug}`}
                      checked={d.weekends}
                      disabled={disabled}
                      onCheckedChange={(weekends) => edit(item, { weekends })}
                    />
                    <div className="grid gap-0.5">
                      <Label htmlFor={`${sectionId}-weekends-${item.slug}`} className="text-xs">
                        Delivers on weekends
                      </Label>
                      <p className="text-xs text-muted-foreground">
                        Off skips Saturday and Sunday in the arrival date shoppers see, so a Friday order quoted 1 day arrives Monday.
                      </p>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </DetailPageSection>
  )
}
