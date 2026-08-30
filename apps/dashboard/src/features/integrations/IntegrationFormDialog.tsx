import { useState, type FormEvent, type ReactNode } from 'react'
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
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { Textarea } from '@/components/ui/textarea'
import { PermissionSelector } from '@/components/PermissionSelector'
import {
  CreateIntegrationDocument,
  IntegrationsDocument,
  UpdateIntegrationDocument,
} from './Integrations.gql'

export type ExistingIntegration = {
  id: string
  name: string
  description: string
  expiresAt?: string | null
  channelId?: string | null
  permissions: { slug: string }[]
}

// Datetime-local input works with strings like "2026-05-01T12:30" - We convert to ISO for the backend - For `null` ISO
// strings (no expiry) the input is empty and the "Set expiry" checkbox stays unchecked
function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function IntegrationFormDialog({
  integration,
  trigger,
  open,
  onOpenChange,
  onCreated,
}: {
  integration?: ExistingIntegration
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onCreated?: (payload: { token: string; name: string }) => void
}) {
  const isEdit = !!integration
  const [internalOpen, setInternalOpen] = useState(false)
  const actualOpen = open ?? internalOpen
  const setOpen = onOpenChange ?? setInternalOpen

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [permissionSlugs, setPermissionSlugs] = useState<string[]>([])
  const [hasExpiry, setHasExpiry] = useState(false)
  const [expiresAt, setExpiresAt] = useState('')
  // '' = global token - the select needs a sentinel since Radix rejects empty item values
  const [channelId, setChannelId] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)
  const channelsQuery = useQuery(ChannelsDocument, { fetchPolicy: 'cache-first' })
  const channels = channelsQuery.data?.channels ?? []

  // Re-hydrate the draft whenever the dialog opens (or the integration
  // changes while open) - adjust state during render instead of an effect
  const [prevOpen, setPrevOpen] = useState(actualOpen)
  const [prevIntegration, setPrevIntegration] = useState(integration)
  if (prevOpen !== actualOpen || prevIntegration !== integration) {
    setPrevOpen(actualOpen)
    setPrevIntegration(integration)
    if (actualOpen) {
      setName(integration?.name ?? '')
      setDescription(integration?.description ?? '')
      setPermissionSlugs(integration?.permissions.map((p) => p.slug) ?? [])
      setChannelId(integration?.channelId ?? '')
      const localExpiry = toLocalInput(integration?.expiresAt)
      setHasExpiry(!!localExpiry)
      setExpiresAt(localExpiry)
      setSubmitError(null)
    }
  }

  const refetchQueries = [{ query: IntegrationsDocument }]
  const [createIntegration, { loading: creating }] = useMutation(CreateIntegrationDocument, {
    refetchQueries,
  })
  const [updateIntegration, { loading: updating }] = useMutation(UpdateIntegrationDocument, {
    refetchQueries,
  })
  const loading = creating || updating

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    try {
      const expiresIso = hasExpiry && expiresAt ? new Date(expiresAt).toISOString() : null
      if (isEdit && integration) {
        await updateIntegration({
          variables: {
            input: {
              id: integration.id,
              name,
              description,
              permissionSlugs,
              // If the user unchecked the expiry, clear it explicitly
              // Otherwise pass the new value (or omit if unchanged & empty)
              ...(hasExpiry
                ? { expiresAt: expiresIso }
                : { clearExpiresAt: true }),
              ...(channelId ? { channelId } : { clearChannel: true }),
            },
          },
        })
      } else {
        const { data } = await createIntegration({
          variables: {
            input: {
              name,
              description,
              permissionSlugs,
              expiresAt: expiresIso,
              channelId: channelId || null,
            },
          },
        })
        if (data) {
          onCreated?.({
            token: data.createIntegration.rawToken,
            name: data.createIntegration.integration.name,
          })
        }
      }
      setOpen(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <Dialog open={actualOpen} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? `Edit ${integration?.name}` : 'New integration'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Update the integration. Changes take effect on the next API call; to rotate credentials use Regenerate token.'
              : 'Create an integration. You\'ll see the API token exactly once after creation.'}
          </DialogDescription>
        </DialogHeader>

        <form id="integration-form" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="integration-name">Name</Label>
            <Input
              id="integration-name"
              required
              placeholder="Order placed webhook"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="integration-description">Description</Label>
            <Textarea
              id="integration-description"
              placeholder="What is this token used for?"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          <div className="grid gap-2">
            <Label>Permissions</Label>
            <PermissionSelector selected={permissionSlugs} onChange={setPermissionSlugs} />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="integration-channel">Channel</Label>
            <Select value={channelId || 'all'} onValueChange={(v) => setChannelId(v === 'all' ? '' : v)}>
              <SelectTrigger id="integration-channel">
                <SelectValue placeholder="All channels" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All channels (global token)</SelectItem>
                {channels.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              A pinned token only satisfies checks for that channel - a feed token for one storefront
              can't read the others.
            </p>
          </div>

          <div className="grid gap-2">
            <div className="flex items-center gap-2">
              <Checkbox
                id="has-expiry"
                checked={hasExpiry}
                onCheckedChange={(v) => setHasExpiry(!!v)}
              />
              <Label htmlFor="has-expiry" className="cursor-pointer">
                Set expiry date
              </Label>
            </div>
            {hasExpiry && (
              <Input
                type="datetime-local"
                value={expiresAt}
                onChange={(e) => setExpiresAt(e.target.value)}
                required
              />
            )}
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
          <Button type="submit" form="integration-form" disabled={loading}>
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create integration'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
