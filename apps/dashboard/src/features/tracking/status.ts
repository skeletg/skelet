import type { TrackingDestinationFieldsFragment, TrackingPlatformFieldsFragment } from './Tracking.gql'

export type Platform = TrackingPlatformFieldsFragment
export type Destination = TrackingDestinationFieldsFragment

export type StatusKind = 'not-installed' | 'off-on-channel' | 'not-set-up' | 'needs-secret' | 'errors' | 'enabled' | 'disabled'

export type Status = { kind: StatusKind; label: string; variant: 'success' | 'warning' | 'destructive' | 'info' | 'outline' }

const STATUS: Record<StatusKind, Status> = {
  'not-installed': { kind: 'not-installed', label: 'Not in this build', variant: 'outline' },
  'off-on-channel': { kind: 'off-on-channel', label: 'Plugin off on this channel', variant: 'warning' },
  'not-set-up': { kind: 'not-set-up', label: 'Not set up', variant: 'outline' },
  'needs-secret': { kind: 'needs-secret', label: 'Needs its secret', variant: 'warning' },
  errors: { kind: 'errors', label: 'Failing', variant: 'destructive' },
  enabled: { kind: 'enabled', label: 'On', variant: 'success' },
  disabled: { kind: 'disabled', label: 'Off', variant: 'info' },
}

export function destinationStatus(d: Destination): Status {
  if (d.needsSecret) return STATUS['needs-secret']
  if (d.enabled && d.health.dead24h > 0) return STATUS.errors
  return d.enabled ? STATUS.enabled : STATUS.disabled
}

// A platform's status on the channel: whether it can be used at all, then the most pressing of its destinations
export function platformStatus(p: Platform, destinations: Destination[]): Status {
  if (!p.installed) return STATUS['not-installed']
  if (!p.availableOnChannel) return STATUS['off-on-channel']
  if (destinations.length === 0) return STATUS['not-set-up']
  const order: StatusKind[] = ['needs-secret', 'errors', 'enabled', 'disabled']
  const statuses = destinations.map(destinationStatus)
  return order.map((k) => statuses.find((s) => s.kind === k)).find(Boolean) ?? STATUS.disabled
}

// Each published plugin lives in a repository named after its slug
export const installCommand = (slug: string) => `skelet cli plugin add github.com/skeletg/${slug}`
