import type { ProductMediaRole } from '@/graphql/schema.types'

export type EffectiveRole = { id: string | null; auto: boolean }

// What the storefront actually renders per role - explicit assignment first, else the same fallback the front API and
// PDP apply (main and OG take the first ready image, thumbnail follows main). Display only: the admin role fields stay
// literal because they are the staged-save baseline
export function effectiveMediaRoles(
  media: ReadonlyArray<{ id: string; status: string }>,
  assigned: Record<ProductMediaRole, string | null>,
): Record<ProductMediaRole, EffectiveRole> {
  const firstReady = media.find((m) => m.status === 'READY')?.id ?? null
  const pick = (id: string | null, fallback: string | null): EffectiveRole =>
    id ? { id, auto: false } : { id: fallback, auto: fallback !== null }
  const main = pick(assigned.MAIN, firstReady)
  return {
    MAIN: main,
    THUMBNAIL: pick(assigned.THUMBNAIL, main.id),
    OG_IMAGE: pick(assigned.OG_IMAGE, firstReady),
  }
}
