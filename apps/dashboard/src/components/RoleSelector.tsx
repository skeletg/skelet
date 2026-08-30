import { useQuery } from '@apollo/client/react'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { RolesDocument } from '@/features/admins/roles/Roles.gql'

// Multi-select role picker, backed by the `roles` query - Callers pass the
// set of selected role slugs and receive the updated set on each toggle
export function RoleSelector({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (next: string[]) => void
}) {
  const { data, loading, error } = useQuery(RolesDocument)
  const selectedSet = new Set(selected)

  const toggle = (slug: string) => {
    const next = new Set(selectedSet)
    if (next.has(slug)) next.delete(slug)
    else next.add(slug)
    onChange([...next])
  }

  if (loading) return <p className="text-sm text-muted-foreground">Loading roles…</p>
  if (error) return <p className="text-sm text-destructive">Error: {error.message}</p>

  const roles = data?.roles ?? []
  if (roles.length === 0) {
    return <p className="text-sm text-muted-foreground">No roles defined. Create one first.</p>
  }

  return (
    <div className="grid grid-cols-1 gap-2 rounded border bg-card p-3 sm:grid-cols-2">
      {roles.map((r) => (
        <div key={r.id} className="flex items-start gap-2">
          <Checkbox
            id={`role-${r.id}`}
            checked={selectedSet.has(r.slug)}
            onCheckedChange={() => toggle(r.slug)}
          />
          <Label htmlFor={`role-${r.id}`} className="flex-1 cursor-pointer">
            <div className="text-sm font-medium">{r.name}</div>
            <code className="text-xs text-muted-foreground">{r.slug}</code>
          </Label>
        </div>
      ))}
    </div>
  )
}
