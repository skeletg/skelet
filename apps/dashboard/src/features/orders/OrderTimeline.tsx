import type { OrderQuery } from './Orders.gql'

type OrderEvent = OrderQuery['order']['events'][number]

// Sidebar from lg up, inline in the body below that - a hidden sidebar would take the audit trail with it
export function OrderTimeline({ events }: { events: readonly OrderEvent[] }) {
  if (events.length === 0) {
    return <p className="text-sm text-muted-foreground">No events recorded.</p>
  }
  return (
    <ol className="space-y-3">
      {[...events]
        .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
        .map((e) => (
          <li key={e.id} className="grid grid-cols-[auto_1fr] gap-3 text-sm">
            <div className="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary/60" />
            <div className="min-w-0">
              <div className="font-medium">{e.message}</div>
              <div className="text-xs text-muted-foreground">
                <span className="font-mono">{e.type}</span>
                {' · '}
                {new Date(e.createdAt).toLocaleString()}
              </div>
            </div>
          </li>
        ))}
    </ol>
  )
}
