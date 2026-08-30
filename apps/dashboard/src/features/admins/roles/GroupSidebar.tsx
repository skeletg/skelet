import { Badge } from '@/components/ui/badge'
import type { GroupRow } from './useRoleDraft'

// Left-column list of permission groups with their "granted X of Y" badge - selection state lives in the parent so the editor pane stays in
// sync, and rows are ghost-button-style so the active one picks up the standard selected-list look without inventing tokens
export function GroupSidebar({
  groups,
  activeGroupID,
  grantedCount,
  onSelect,
}: {
  groups: GroupRow[]
  activeGroupID: string | null
  grantedCount: Record<string, number>
  onSelect: (groupID: string) => void
}) {
  return (
    <nav className="grid gap-0.5" aria-label="Permission groups">
      {groups.map((group) => {
        const active = group.id === activeGroupID
        const granted = grantedCount[group.id] ?? 0
        const total = group.permissions.length
        return (
          <button
            key={group.id}
            type="button"
            onClick={() => onSelect(group.id)}
            className={`flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-left text-sm transition-colors ${
              active
                ? 'border-primary bg-primary/10 text-foreground'
                : 'border-transparent hover:bg-muted/40 text-muted-foreground hover:text-foreground'
            }`}
          >
            <span className="truncate font-medium">{group.name}</span>
            <Badge
              variant={granted > 0 ? 'secondary' : 'outline'}
              className="shrink-0 font-mono text-[10px]"
              title={`${granted} of ${total} permission${total === 1 ? '' : 's'} granted`}
            >
              {granted}/{total}
            </Badge>
          </button>
        )
      })}
    </nav>
  )
}
