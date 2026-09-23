import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
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
import { Textarea } from '@/components/ui/textarea'
import {
  CreateCustomerGroupDocument,
  CustomerGroupsDocument,
  UpdateCustomerGroupDocument,
} from './CustomerGroups.gql'

export type ExistingCustomerGroup = {
  id: string
  slug: string
  name: string
  description: string
}

export function CustomerGroupFormDialog({
  group,
  trigger,
  open,
  onOpenChange,
}: {
  group?: ExistingCustomerGroup
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const isEdit = !!group
  const [internalOpen, setInternalOpen] = useState(false)
  const actualOpen = open ?? internalOpen
  const setOpen = onOpenChange ?? setInternalOpen

  const [slug, setSlug] = useState(group?.slug ?? '')
  const [name, setName] = useState(group?.name ?? '')
  const [description, setDescription] = useState(group?.description ?? '')
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Re-hydrate the draft whenever the dialog opens (or the group changes
  // while open) - adjust state during render instead of an effect
  const [prevOpen, setPrevOpen] = useState(actualOpen)
  const [prevGroup, setPrevGroup] = useState(group)
  if (prevOpen !== actualOpen || prevGroup !== group) {
    setPrevOpen(actualOpen)
    setPrevGroup(group)
    if (actualOpen) {
      setSlug(group?.slug ?? '')
      setName(group?.name ?? '')
      setDescription(group?.description ?? '')
      setSubmitError(null)
    }
  }

  const refetchQueries = [{ query: CustomerGroupsDocument }]
  const [createGroup, { loading: creating }] = useMutation(CreateCustomerGroupDocument, { refetchQueries })
  const [updateGroup, { loading: updating }] = useMutation(UpdateCustomerGroupDocument, { refetchQueries })
  const loading = creating || updating

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    try {
      if (isEdit && group) {
        await updateGroup({
          variables: {
            id: group.id,
            input: { name, description: description || null },
          },
        })
      } else {
        await createGroup({
          variables: { input: { slug, name, description: description || null } },
        })
      }
      setOpen(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <Dialog open={actualOpen} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? `Edit ${group?.name}` : 'New customer group'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Update the group. Slug is immutable - create a new group instead if you need a different slug.'
              : 'Create a customer group. Customers can be assigned to it individually.'}
          </DialogDescription>
        </DialogHeader>

        <form id="customer-group-form" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
            <div className="grid gap-2">
              <Label htmlFor="cg-slug">Slug</Label>
              <Input
                id="cg-slug"
                required
                disabled={isEdit}
                placeholder="wholesale"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
              />
              {isEdit && <p className="text-xs text-muted-foreground">Slug is immutable.</p>}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="cg-name">Name</Label>
              <Input
                id="cg-name"
                required
                placeholder="Wholesale"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="cg-description">Description</Label>
            <Textarea
              id="cg-description"
              placeholder="Who belongs in this group?"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Save failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button type="submit" form="customer-group-form" disabled={loading}>
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create group'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
