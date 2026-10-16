import { added, fieldDiff, removed, sameJson } from '@/shared/saving/diff'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { MODE_LABELS } from './labels'
import { blockMode, type Configuration, type Mode, type Option, type Point } from './types'

// What a merchant reads for an option's settings - names, not slugs
function optionSpecs(config: Configuration) {
  const kind = (o: Option) => config.kinds.find((k) => k.slug === o.kind)
  const sourceName = (slug: string) => (slug ? (config.pointSources.find((s) => s.slug === slug)?.label ?? slug) : 'Default')
  const carrierName = (o: Option) =>
    o.carrierSlug
      ? (config.carrierKinds.find((c) => c.kind === o.kind)?.carrierRefs.find((r) => r.slug === o.carrierSlug)?.name ?? o.carrierSlug)
      : 'Any carrier'
  return (label: string, o: Option) => {
    const blocks = (kind(o)?.blocks ?? []).filter((b) => b.floor === 'CONFIGURABLE')
    return [
      { label: `${label} · Name`, value: (x: Option) => x.name },
      { label: `${label} · Description`, value: (x: Option) => x.description },
      { label: `${label} · Display order`, value: (x: Option) => x.sortOrder },
      { label: `${label} · Enabled`, value: (x: Option) => x.enabled },
      ...blocks.map((b) => ({
        label: `${label} · ${b.label}`,
        value: (x: Option) => MODE_LABELS[blockMode(b, x.blockModes) as Mode],
      })),
      { label: `${label} · Location source`, value: (x: Option) => sourceName(x.pointSource) },
      { label: `${label} · Carrier`, value: carrierName },
    ]
  }
}

export function optionsDiff(before: Option[], after: Option[], deleted: string[], config: Configuration): SectionDiffEntry[] {
  const out: SectionDiffEntry[] = []
  const specs = optionSpecs(config)
  const kindLabel = (o: Option) => config.kinds.find((k) => k.slug === o.kind)?.label ?? o.kind
  for (const o of after) {
    const label = `Delivery · ${o.name}`
    const old = before.find((x) => x.id === o.id)
    if (!old) out.push(added(label, `${kindLabel(o)} · ${o.enabled ? 'Offer after saving' : 'Off'}`))
    else if (!sameJson(old, o)) out.push(...fieldDiff(old, o, specs(label, o)))
  }
  for (const id of deleted) {
    const old = before.find((x) => x.id === id)
    if (old) out.push(removed(`Delivery · ${old.name}`, kindLabel(old)))
  }
  return out
}

const pointAddress = (p: Point) => [p.street1, p.street2, p.postalCode, p.city, p.region, p.countryCode].filter(Boolean).join(', ')

export function pointsDiff(before: Point[], after: Point[]): SectionDiffEntry[] {
  const out: SectionDiffEntry[] = []
  for (const p of after) {
    const label = `Pickup point · ${p.name || 'New pickup point'}`
    const old = before.find((x) => x.key === p.key)
    if (!old) {
      out.push(added(label, pointAddress(p)))
      continue
    }
    out.push(
      ...fieldDiff(old, p, [
        { label: `${label} · Name`, value: (x: Point) => x.name },
        { label: `${label} · Address`, value: pointAddress },
        { label: `${label} · Instructions`, value: (x: Point) => x.instructions },
        { label: `${label} · Hours`, value: (x: Point) => x.hours },
        { label: `${label} · Active`, value: (x: Point) => x.isActive },
      ]),
    )
  }
  for (const old of before) {
    if (!after.some((p) => p.key === old.key)) out.push(removed(`Pickup point · ${old.name}`, pointAddress(old)))
  }
  return out
}
