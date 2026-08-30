import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
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
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RoleSelector } from '@/components/RoleSelector'
import { AdminsDocument, CreateAdminDocument, UpdateAdminDocument } from './Admins.gql'

export type ExistingAdmin = {
  id: string
  email: string
  firstName: string
  lastName: string
  isActive: boolean
  isSuperuser: boolean
  roles: { slug: string }[]
}

export function AdminFormDialog({
  admin,
  trigger,
  open,
  onOpenChange,
}: {
  admin?: ExistingAdmin
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const isEdit = !!admin
  const [internalOpen, setInternalOpen] = useState(false)
  const actualOpen = open ?? internalOpen
  const setOpen = onOpenChange ?? setInternalOpen

  const [firstName, setFirstName] = useState(admin?.firstName ?? '')
  const [lastName, setLastName] = useState(admin?.lastName ?? '')
  const [email, setEmail] = useState(admin?.email ?? '')
  const [password, setPassword] = useState('')
  const [isActive, setIsActive] = useState(admin?.isActive ?? true)
  const [roleSlugs, setRoleSlugs] = useState<string[]>(
    admin?.roles.map((r) => r.slug) ?? [],
  )
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Reset the form when the dialog (re)opens or the target admin changes
  // while open (adjust-state-during-render)
  const [prevOpen, setPrevOpen] = useState(actualOpen)
  const [prevAdmin, setPrevAdmin] = useState(admin)
  if (prevOpen !== actualOpen || prevAdmin !== admin) {
    setPrevOpen(actualOpen)
    setPrevAdmin(admin)
    if (actualOpen) {
      setFirstName(admin?.firstName ?? '')
      setLastName(admin?.lastName ?? '')
      setEmail(admin?.email ?? '')
      setPassword('')
      setIsActive(admin?.isActive ?? true)
      setRoleSlugs(admin?.roles.map((r) => r.slug) ?? [])
      setSubmitError(null)
    }
  }

  const [createAdmin, { loading: creating }] = useMutation(CreateAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })
  const [updateAdmin, { loading: updating }] = useMutation(UpdateAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })
  const loading = creating || updating

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    try {
      if (isEdit && admin) {
        await updateAdmin({
          variables: {
            id: admin.id,
            input: { firstName, lastName, isActive, roleSlugs },
          },
        })
      } else {
        await createAdmin({
          variables: { input: { firstName, lastName, email, password, roleSlugs } },
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
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? `Edit ${admin?.firstName} ${admin?.lastName}`.trim() : 'New admin'}
          </DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Update the admin\'s profile, roles, and active status. Email and password can\'t be changed here.'
              : 'The admin will be able to sign in immediately with the password you set here. Share credentials over a secure channel.'}
          </DialogDescription>
        </DialogHeader>

        <form id="admin-form" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="admin-first">First name</Label>
              <Input
                id="admin-first"
                required
                autoComplete="given-name"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="admin-last">Last name</Label>
              <Input
                id="admin-last"
                required
                autoComplete="family-name"
                value={lastName}
                onChange={(e) => setLastName(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="admin-email">Email</Label>
            <Input
              id="admin-email"
              type="email"
              required
              disabled={isEdit}
              autoComplete="off"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            {isEdit && (
              <p className="text-xs text-muted-foreground">Email is immutable.</p>
            )}
          </div>

          {!isEdit && (
            <div className="grid gap-2">
              <Label htmlFor="admin-password">Password</Label>
              <Input
                id="admin-password"
                type="password"
                required
                minLength={8}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">Minimum 8 characters.</p>
            </div>
          )}

          {isEdit && (
            <div className="flex items-center gap-2">
              <Checkbox
                id="admin-active"
                checked={isActive}
                onCheckedChange={(v) => setIsActive(!!v)}
              />
              <Label htmlFor="admin-active" className="cursor-pointer">
                Active - inactive admins cannot sign in
              </Label>
            </div>
          )}

          {isEdit && admin?.isSuperuser ? (
            <div className="grid gap-2">
              <Label>Roles</Label>
              <p className="rounded border bg-muted p-3 text-sm text-muted-foreground">
                This admin is a superuser and bypasses role-based permissions. Role assignments
                have no effect.
              </p>
            </div>
          ) : (
            <div className="grid gap-2">
              <Label>Roles</Label>
              <RoleSelector selected={roleSlugs} onChange={setRoleSlugs} />
            </div>
          )}

          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>{isEdit ? 'Couldn\'t update admin' : 'Couldn\'t create admin'}</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button type="submit" form="admin-form" disabled={loading}>
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create admin'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
