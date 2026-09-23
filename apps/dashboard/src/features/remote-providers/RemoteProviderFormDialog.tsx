import { useState, type ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type {
  RemoteProviderFailurePolicy,
  RemoteProviderKind,
} from '@/graphql/schema.types'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { KIND_HINTS, KIND_LABELS } from './kinds'
import {
  CreateRemoteProviderDocument,
  RemoteProvidersDocument,
  UpdateRemoteProviderDocument,
  type UpdateRemoteProviderMutationVariables,
} from './RemoteProviders.gql'

export type ExistingProvider = {
  id: string
  kind: RemoteProviderKind
  slug: string
  name: string
  targetUrl: string
  config?: unknown
  timeoutMs: number
  cacheTtlS: number
  failurePolicy: RemoteProviderFailurePolicy
  channel?: { id: string; slug: string; name: string } | null
}

const ALL_STORES_VALUE = '__all__'

export function RemoteProviderFormDialog({
  trigger,
  existing,
  onCreated,
  onUpdated,
}: {
  trigger: ReactNode
  existing?: ExistingProvider
  onCreated?: (data: { secret: string; name: string }) => void
  onUpdated?: () => void
}) {
  const [open, setOpen] = useState(false)
  const [kind, setKind] = useState<RemoteProviderKind>('SHIPPING_CARRIER')
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [targetUrl, setTargetUrl] = useState('')
  const [configText, setConfigText] = useState('')
  const [timeoutMs, setTimeoutMs] = useState('')
  const [cacheTtlS, setCacheTtlS] = useState('')
  const [failurePolicy, setFailurePolicy] = useState<RemoteProviderFailurePolicy>('FAIL_OPEN')
  const [channelId, setChannelId] = useState(ALL_STORES_VALUE)
  const [error, setError] = useState<string | null>(null)

  const channelsQuery = useQuery(ChannelsDocument, { skip: !open })
  const channels = channelsQuery.data?.channels ?? []
  const [createProvider, { loading: creating }] = useMutation(CreateRemoteProviderDocument, {
    refetchQueries: [{ query: RemoteProvidersDocument }],
  })
  const [updateProvider, { loading: updating }] = useMutation(UpdateRemoteProviderDocument)

  // hydrate on open (state adjustment during render, same pattern as
  // WebhookFormDialog)
  const [prevKey, setPrevKey] = useState<{ open: boolean; existing?: ExistingProvider }>({
    open,
    existing,
  })
  if (prevKey.open !== open || prevKey.existing !== existing) {
    setPrevKey({ open, existing })
    if (open) {
      setKind(existing?.kind ?? 'SHIPPING_CARRIER')
      setSlug(existing?.slug ?? '')
      setName(existing?.name ?? '')
      setTargetUrl(existing?.targetUrl ?? '')
      setConfigText(existing?.config ? JSON.stringify(existing.config, null, 2) : '')
      setTimeoutMs(existing?.timeoutMs ? String(existing.timeoutMs) : '')
      setCacheTtlS(existing?.cacheTtlS ? String(existing.cacheTtlS) : '')
      setFailurePolicy(existing?.failurePolicy ?? 'FAIL_OPEN')
      setChannelId(existing?.channel?.id ?? ALL_STORES_VALUE)
      setError(null)
    }
  }

  function parsedConfig(): unknown | undefined {
    const text = configText.trim()
    if (!text) return undefined
    return JSON.parse(text)
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    let config: unknown | undefined
    try {
      config = parsedConfig()
    } catch {
      setError('Config must be valid JSON.')
      return
    }
    try {
      if (existing) {
        const input: UpdateRemoteProviderMutationVariables['input'] = { id: existing.id }
        if (name !== existing.name) input.name = name
        if (targetUrl !== existing.targetUrl) input.targetUrl = targetUrl
        const oldConfig = existing.config ? JSON.stringify(existing.config) : ''
        const newConfig = config === undefined ? '' : JSON.stringify(config)
        if (newConfig !== oldConfig) input.config = config ?? {}
        const t = timeoutMs.trim() === '' ? 0 : Number(timeoutMs)
        if (t !== existing.timeoutMs) input.timeoutMs = t
        const ttl = cacheTtlS.trim() === '' ? 0 : Number(cacheTtlS)
        if (ttl !== existing.cacheTtlS) input.cacheTtlS = ttl
        if (failurePolicy !== existing.failurePolicy) input.failurePolicy = failurePolicy
        if (Object.keys(input).length > 1) {
          await updateProvider({ variables: { input } })
        }
        setOpen(false)
        onUpdated?.()
      } else {
        const { data } = await createProvider({
          variables: {
            input: {
              kind,
              slug,
              name,
              targetUrl,
              config,
              timeoutMs: timeoutMs.trim() === '' ? undefined : Number(timeoutMs),
              cacheTtlS: cacheTtlS.trim() === '' ? undefined : Number(cacheTtlS),
              failurePolicy,
              channelId: channelId === ALL_STORES_VALUE ? undefined : channelId,
            },
          },
        })
        setOpen(false)
        if (data) {
          onCreated?.({
            secret: data.createRemoteProvider.rawSecret,
            name: data.createRemoteProvider.provider.name,
          })
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed.')
    }
  }

  const isLoading = creating || updating

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-lg">
        <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{existing ? 'Edit remote provider' : 'New remote provider'}</DialogTitle>
            <DialogDescription>
              {existing
                ? 'Update this endpoint. Kind, slug and store scope are fixed at creation.'
                : 'Register an HTTP endpoint that implements a pluggable role. Skelet calls it mid-request with a signed JSON envelope; its response is the answer.'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto py-1 pr-1">
            <div className="grid gap-2">
              <Label htmlFor="rp-kind">Role</Label>
              <Select
                value={kind}
                onValueChange={(v) => setKind(v as RemoteProviderKind)}
                disabled={!!existing}
              >
                <SelectTrigger id="rp-kind">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(KIND_LABELS) as RemoteProviderKind[]).map((k) => (
                    <SelectItem key={k} value={k}>
                      {KIND_LABELS[k]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{KIND_HINTS[kind]}</p>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="rp-slug">Slug</Label>
                <Input
                  id="rp-slug"
                  placeholder="my-tax-service"
                  value={slug}
                  onChange={(e) => setSlug(e.target.value)}
                  disabled={!!existing}
                  required
                  className="font-mono"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="rp-name">Name</Label>
                <Input
                  id="rp-name"
                  placeholder="My tax service"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                />
              </div>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="rp-url">Endpoint URL</Label>
              <Input
                id="rp-url"
                type="url"
                placeholder="https://api.example.com/skelet/tax"
                value={targetUrl}
                onChange={(e) => setTargetUrl(e.target.value)}
                required
                className="font-mono"
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="rp-store">Channel scope</Label>
              <Select
                value={channelId}
                onValueChange={setChannelId}
                disabled={channelsQuery.loading || !!existing}
              >
                <SelectTrigger id="rp-store">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL_STORES_VALUE}>All stores (global)</SelectItem>
                  {channels.map((ch) => (
                    <SelectItem key={ch.id} value={ch.id}>
                      {ch.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="rp-config">Config (JSON, optional)</Label>
              <Textarea
                id="rp-config"
                placeholder='{"apiKey": "..."}'
                value={configText}
                onChange={(e) => setConfigText(e.target.value)}
                rows={4}
                className="font-mono text-xs"
              />
              <p className="text-xs text-muted-foreground">
                Forwarded verbatim in every invocation envelope - receiver-side keys and options.
              </p>
            </div>

            <div className="grid gap-4 sm:grid-cols-3">
              <div className="grid gap-2">
                <Label htmlFor="rp-timeout">Timeout (ms)</Label>
                <Input
                  id="rp-timeout"
                  type="number"
                  min={0}
                  max={30000}
                  placeholder="Default: 5000"
                  value={timeoutMs}
                  onChange={(e) => setTimeoutMs(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="rp-cache">Cache TTL (s)</Label>
                <Input
                  id="rp-cache"
                  type="number"
                  min={0}
                  max={3600}
                  placeholder="0 = off"
                  value={cacheTtlS}
                  onChange={(e) => setCacheTtlS(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="rp-policy">On failure</Label>
                <Select
                  value={failurePolicy}
                  onValueChange={(v) => setFailurePolicy(v as RemoteProviderFailurePolicy)}
                >
                  <SelectTrigger className="w-full" id="rp-policy">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="FAIL_OPEN">Fall back to core</SelectItem>
                    <SelectItem value="FAIL_CLOSED">Abort the operation</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading ? 'Saving…' : existing ? 'Save changes' : 'Create provider'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
