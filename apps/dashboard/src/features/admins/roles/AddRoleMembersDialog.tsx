import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AdminsDocument, UpdateAdminDocument } from '@/features/admins/Admins.gql'
import { RoleDocument } from '@/features/admins/roles/Roles.gql'

type Admin = {
  id: string
  email: string
  firstName: string
  lastName: string
  isSuperuser: boolean
  roles: { id: string; slug: string; name: string }[]
}

// Assigning admins to a role is done via `updateAdmin` - the backend replaces the admin's full roleSlugs set, so we
// fetch the admin's current roles and add the target role slug on top before issuing the mutation
export function AddRoleMembersDialog({
  roleId,
  roleSlug,
  roleName,
  open,
  onOpenChange,
}: {
  roleId: string
  roleSlug: string
  roleName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { data, loading, error } = useQuery(AdminsDocument, { skip: !open })
  const [updateAdmin] = useMutation(UpdateAdminDocument, {
    refetchQueries: [
      { query: AdminsDocument },
      { query: RoleDocument, variables: { id: roleId } },
    ],
  })

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [filter, setFilter] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const eligible = useMemo<Admin[]>(() => {
    const all = (data?.admins ?? []) as Admin[]
    // Skip superusers (they bypass roles) and admins already in this role
    return all
      .filter((a) => !a.isSuperuser && !a.roles.some((r) => r.slug === roleSlug))
      .filter((a) => {
        if (!filter) return true
        const q = filter.toLowerCase()
        return (
          a.email.toLowerCase().includes(q) ||
          `${a.firstName} ${a.lastName}`.toLowerCase().includes(q)
        )
      })
  }, [data, roleSlug, filter])

  const toggle = (id: string) => {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelected(next)
  }

  const reset = () => {
    setSelected(new Set())
    setFilter('')
    setSubmitError(null)
  }

  async function onAssign() {
    if (selected.size === 0) return
    setSubmitting(true)
    setSubmitError(null)
    try {
      const all = (data?.admins ?? []) as Admin[]
      await Promise.all(
        [...selected].map((id) => {
          const admin = all.find((a) => a.id === id)
          if (!admin) return Promise.resolve()
          const nextSlugs = Array.from(
            new Set([...admin.roles.map((r) => r.slug), roleSlug]),
          )
          return updateAdmin({ variables: { id, input: { roleSlugs: nextSlugs } } })
        }),
      )
      reset()
      onOpenChange(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Assignment failed.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) reset()
        onOpenChange(v)
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add members to {roleName}</DialogTitle>
          <DialogDescription>
            Superusers are hidden (they bypass role checks) and admins already in this role
            don't appear.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-3">
          <Input
            placeholder="Filter by name or email"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />

          {loading && <p className="text-sm text-muted-foreground">Loading admins…</p>}
          {error && <p className="text-sm text-destructive">{error.message}</p>}

          {data && (
            <div className="max-h-80 overflow-y-auto rounded border">
              {eligible.length === 0 ? (
                <p className="p-4 text-sm text-muted-foreground">
                  No eligible admins.
                </p>
              ) : (
                <ul className="divide-y">
                  {eligible.map((a) => (
                    <li key={a.id} className="flex items-center gap-3 p-3">
                      <Checkbox
                        id={`pick-${a.id}`}
                        checked={selected.has(a.id)}
                        onCheckedChange={() => toggle(a.id)}
                      />
                      <Label htmlFor={`pick-${a.id}`} className="flex-1 cursor-pointer">
                        <div className="text-sm font-medium">
                          {a.firstName} {a.lastName}
                        </div>
                        <div className="text-xs text-muted-foreground">{a.email}</div>
                      </Label>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}

          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Couldn't add members</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={onAssign} disabled={selected.size === 0 || submitting}>
            {submitting ? 'Adding…' : `Add ${selected.size || ''}`.trim()}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
