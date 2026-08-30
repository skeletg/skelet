import { useState, type ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  CmsAttributeGroupsDocument,
  CreateCmsAttributeGroupDocument,
  DeleteCmsAttributeGroupDocument,
  UpdateCmsAttributeGroupDocument,
} from './Cms.gql'

// Attribute groups are the field-grouping layer for page-type forms: when an attribute is assigned to a page type it can be filed under a
// group, which is how the page editor lays fields out - The groups themselves are global and had no UI, so groupId was unusable

type GroupRow = { id: string; slug: string; position: number }

export function CmsAttributeGroupsDialog({ trigger }: { trigger: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [newSlug, setNewSlug] = useState('')
  const [edits, setEdits] = useState<Record<string, string>>({})
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const { data, loading, refetch } = useQuery(CmsAttributeGroupsDocument, {
    skip: !open,
    fetchPolicy: 'cache-and-network',
  })
  const [createGroup] = useMutation(CreateCmsAttributeGroupDocument)
  const [updateGroup] = useMutation(UpdateCmsAttributeGroupDocument)
  const [deleteGroup] = useMutation(DeleteCmsAttributeGroupDocument)

  const groups = (data?.cmsAttributeGroups ?? []) as GroupRow[]

  async function run(fn: () => Promise<unknown>) {
    setError(null)
    setBusy(true)
    try {
      await fn()
      await refetch()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Attribute groups</DialogTitle>
          <DialogDescription>
            Groups organise fields inside a page type&rsquo;s form. Assign an attribute to a group
            when you add it to a page type.
          </DialogDescription>
        </DialogHeader>

        {error ? (
          <Alert variant="destructive">
            <AlertTitle>Failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        <div className="grid gap-2">
          {loading && groups.length === 0 ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : groups.length === 0 ? (
            <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              No groups yet. Add one below.
            </p>
          ) : (
            groups.map((g) => (
              <div key={g.id} className="grid grid-cols-[1fr_auto] items-center gap-2">
                <Input
                  value={edits[g.id] ?? g.slug}
                  className="font-mono"
                  onChange={(e) => setEdits((p) => ({ ...p, [g.id]: e.target.value }))}
                  onBlur={() => {
                    const next = (edits[g.id] ?? g.slug).trim()
                    if (!next || next === g.slug) return
                    void run(() =>
                      updateGroup({
                        variables: { id: g.id, slug: next, position: g.position },
                      }),
                    )
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8 text-destructive"
                  disabled={busy}
                  title="Delete group"
                  onClick={() => void run(() => deleteGroup({ variables: { id: g.id } }))}
                >
                  <Trash2Icon className="size-3.5" />
                </Button>
              </div>
            ))
          )}
        </div>

        <div className="grid grid-cols-[1fr_auto] items-end gap-2 border-t pt-4">
          <div className="grid gap-1.5">
            <Label htmlFor="new-group-slug">New group slug</Label>
            <Input
              id="new-group-slug"
              className="font-mono"
              placeholder="seo"
              value={newSlug}
              onChange={(e) => setNewSlug(e.target.value)}
            />
          </div>
          <Button
            type="button"
            disabled={!newSlug.trim() || busy}
            onClick={() =>
              void run(async () => {
                await createGroup({
                  variables: { slug: newSlug.trim(), position: groups.length },
                })
                setNewSlug('')
              })
            }
          >
            <PlusIcon className="size-4" /> Add
          </Button>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
