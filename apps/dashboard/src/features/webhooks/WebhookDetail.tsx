import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  CalendarIcon,
  CheckCircle2Icon,
  EraserIcon,
  InfoIcon,
  RefreshCwIcon,
  SettingsIcon,
  Trash2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import { WebhookFormDialog } from './WebhookFormDialog'
import { WebhookKindBadge } from './WebhookKindBadge'
import { WebhookHealth } from './WebhookHealth'
import { NewSecretDialog } from './NewSecretDialog'
import { DeliveryLog } from './DeliveryLog'
import { DeadLetters } from './DeadLetters'
import {
  ClearWebhookDeliveriesDocument,
  DeleteWebhookDocument,
  RotateWebhookSecretDocument,
  SetWebhookActiveDocument,
  WebhookDeliveriesDocument,
  WebhookDocument,
  WebhooksDocument,
} from './Webhooks.gql'
import { PluginSections } from '@/plugins/detail-sections'

// WebhookDetail is a standalone PDP-style page reached from /webhooks/:id The table list at /webhooks links each row
// here; the header carries a "Back to webhooks" affordance via DetailPageHeader's backTo
export function WebhookDetail() {
  const params = useParams<{ id?: string }>()
  const id = params.id ?? ''
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('webhooks.manage')

  const { data, loading, error, refetch } = useQuery(WebhookDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })

  const [setActive, { loading: toggling }] = useMutation(SetWebhookActiveDocument)
  const [rotateSecret, { loading: rotating }] = useMutation(RotateWebhookSecretDocument)
  const [deleteWebhook, { loading: deleting }] = useMutation(DeleteWebhookDocument, {
    refetchQueries: [{ query: WebhooksDocument }],
  })
  const [clearDeliveries, { loading: clearing }] = useMutation(ClearWebhookDeliveriesDocument, {
    refetchQueries: [WebhookDeliveriesDocument],
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [confirmingRotate, setConfirmingRotate] = useState(false)
  const [confirmingClear, setConfirmingClear] = useState(false)
  const [clearedCount, setClearedCount] = useState<number | null>(null)
  const [newSecret, setNewSecret] = useState<{ secret: string; name: string } | null>(null)
  const [opError, setOpError] = useState<string | null>(null)

  const webhook = data?.webhook

  async function onToggleActive(active: boolean) {
    if (!webhook) return
    setOpError(null)
    try {
      await setActive({ variables: { id: webhook.id, active } })
      refetch()
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Toggle failed.')
    }
  }

  async function onRotateSecret() {
    if (!webhook) return
    setOpError(null)
    try {
      const { data } = await rotateSecret({ variables: { id: webhook.id } })
      if (data) {
        setNewSecret({
          secret: data.rotateWebhookSecret.rawSecret,
          name: webhook.name,
        })
      }
      setConfirmingRotate(false)
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Rotate failed.')
    }
  }

  async function onDelete() {
    if (!webhook) return
    setOpError(null)
    try {
      await deleteWebhook({ variables: { id: webhook.id } })
      navigate('/webhooks', { replace: true })
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  async function onClearDeliveries() {
    if (!webhook) return
    setOpError(null)
    try {
      const { data } = await clearDeliveries({ variables: { webhookId: webhook.id } })
      const n = data?.clearWebhookDeliveries ?? 0
      setClearedCount(n)
      setTimeout(() => setClearedCount(null), 4000)
      setConfirmingClear(false)
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Clear failed.')
      setConfirmingClear(false)
    }
  }

  if (loading && !webhook) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading webhook…</p>
  }
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load webhook"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!webhook) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Webhook not found.</p>
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/webhooks"
        backLabel="Back to webhooks"
        title={webhook.name}
        badges={<WebhookKindBadge kind={webhook.kind} className="shrink-0" />}
        meta={<code className="font-mono">{webhook.targetUrl}</code>}
        actions={
          <>
            <label
              htmlFor="webhook-active-header"
              className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1 text-sm"
            >
              <Switch
                id="webhook-active-header"
                checked={webhook.isActive}
                disabled={toggling || !canManage}
                onCheckedChange={onToggleActive}
              />
              <span className={webhook.isActive ? 'font-medium' : 'text-muted-foreground'}>
                {webhook.isActive ? 'Active' : 'Inactive'}
              </span>
            </label>
            {canManage && (
              <>
                <WebhookFormDialog
                  trigger={
                    <Button variant="outline" size="sm">
                      <SettingsIcon className="size-4" />
                      Edit
                    </Button>
                  }
                  existing={webhook}
                  onUpdated={refetch}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => { setOpError(null); setConfirmingDelete(true) }}
                  disabled={deleting}
                >
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
              </>
            )}
          </>
        }
      />

      <DetailPageBody>
        {opError && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Operation failed</AlertTitle>
              <AlertDescription>{opError}</AlertDescription>
            </Alert>
          </div>
        )}

        {webhook.kind === 'SYNC' && webhook.circuit && webhook.circuit.state !== 'CLOSED' && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Circuit breaker is {webhook.circuit.state === 'OPEN' ? 'open' : 'half-open'}</AlertTitle>
              <AlertDescription>
                The endpoint failed {webhook.circuit.consecutiveFailures} consecutive deliveries
                {webhook.circuit.openedAt
                  ? ` (open since ${new Date(webhook.circuit.openedAt).toLocaleTimeString()})`
                  : ''}
                . Calls are {webhook.failurePolicy === 'FAIL_OPEN' ? 'skipped' : 'rejected without contacting it'} until
                a probe succeeds. Saving the webhook resets the circuit.
              </AlertDescription>
            </Alert>
          </div>
        )}

        <div className="px-6 py-6">
          <Tabs defaultValue="settings" className="w-full">
            <TabsList className="grid w-full grid-cols-2 lg:w-[400px]">
              <TabsTrigger value="settings">Settings</TabsTrigger>
              <TabsTrigger value="deliveries">Deliveries</TabsTrigger>
            </TabsList>

            <TabsContent value="settings" className="mt-4">
              <div className="divide-y">
                <DetailPageSection
                  title="Delivery health"
                  description="Aggregate outcome of this webhook's delivery attempts."
                  className="px-0"
                >
                  <WebhookHealth webhookId={webhook.id} />
                </DetailPageSection>

                <DetailPageSection
                  title="Webhook details"
                  description="Configuration and event subscriptions."
                  className="px-0"
                >
                  <div className="grid gap-4 text-sm">
                    <div className="grid gap-1">
                      <span className="text-muted-foreground">Target URL</span>
                      <span className="font-mono">{webhook.targetUrl}</span>
                    </div>
                    <div className="grid gap-1">
                      <span className="text-muted-foreground">Channel scope</span>
                      <div>
                        {webhook.channel ? (
                          <Badge variant="secondary">{webhook.channel.name}</Badge>
                        ) : (
                          <Badge variant="outline">All stores (global)</Badge>
                        )}
                      </div>
                    </div>
                    <div className="grid gap-1">
                      <span className="text-muted-foreground">Delivery contract</span>
                      <div className="flex flex-wrap items-center gap-1.5">
                        {webhook.kind === 'SYNC' && (
                          <Badge
                            variant={webhook.failurePolicy === 'FAIL_OPEN' ? 'secondary' : 'outline'}
                          >
                            {webhook.failurePolicy === 'FAIL_OPEN'
                              ? 'Fail open (advisory)'
                              : 'Fail closed (gate)'}
                          </Badge>
                        )}
                        <Badge variant="outline">
                          Timeout:{' '}
                          {webhook.timeoutMs > 0
                            ? `${webhook.timeoutMs}ms`
                            : `default (${webhook.kind === 'SYNC' ? '5s' : '10s'})`}
                        </Badge>
                        <Badge variant="outline">Payload {webhook.payloadVersion}</Badge>
                        {webhook.kind === 'SYNC' && webhook.circuit && (
                          <Badge
                            variant={webhook.circuit.state === 'CLOSED' ? 'outline' : 'destructive'}
                          >
                            Circuit: {webhook.circuit.state.toLowerCase().replace('_', '-')}
                          </Badge>
                        )}
                      </div>
                    </div>
                    {(() => {
                      const headers = Object.entries(
                        (webhook.customHeaders as Record<string, string> | null | undefined) ?? {},
                      )
                      if (headers.length === 0) return null
                      return (
                        <div className="grid gap-1">
                          <span className="text-muted-foreground">Custom headers</span>
                          <div className="grid gap-1 pt-1">
                            {headers.map(([k, v]) => (
                              <code key={k} className="font-mono text-xs">
                                {k}: {v}
                              </code>
                            ))}
                          </div>
                        </div>
                      )
                    })()}
                    <div className="grid gap-1">
                      <span className="text-muted-foreground">
                        Subscribed events ({webhook.eventTypes.length})
                      </span>
                      <div className="flex flex-wrap gap-1.5 pt-1">
                        {webhook.eventTypes.map((et) => (
                          <Badge key={et} variant="secondary" className="text-[10px] uppercase tracking-wider">
                            {et.replace(/\./g, ' ')}
                          </Badge>
                        ))}
                      </div>
                    </div>
                    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 pt-2 text-muted-foreground">
                      <div className="flex items-center gap-2">
                        <CalendarIcon className="size-4" />
                        <span>Created {new Date(webhook.createdAt).toLocaleDateString()}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <RefreshCwIcon className="size-4" />
                        <span>Updated {new Date(webhook.updatedAt).toLocaleString()}</span>
                      </div>
                    </div>
                  </div>
                </DetailPageSection>

                <DetailPageSection
                  title="Authentication"
                  description="Secure your webhook deliveries."
                  className="px-0"
                >
                  <div className="grid gap-4 text-sm">
                    <Alert>
                      <InfoIcon className="size-4" />
                      <AlertTitle className="text-xs font-bold uppercase tracking-wider">How it works</AlertTitle>
                      <AlertDescription className="text-xs">
                        We send an <code className="text-xs">X-Webhook-Signature-256</code> header.
                        Verify it using <code className="text-xs">HMAC-SHA256(secret, body)</code>.
                      </AlertDescription>
                    </Alert>
                    <div className="flex flex-col gap-3">
                      <span className="text-muted-foreground">
                        The secret was shown when the webhook was created. You can rotate it if it's compromised.
                      </span>
                      {canManage && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="w-fit"
                          onClick={() => { setOpError(null); setConfirmingRotate(true) }}
                          disabled={rotating}
                        >
                          <RefreshCwIcon className="size-4" />
                          Rotate secret
                        </Button>
                      )}
                    </div>
                  </div>
                </DetailPageSection>
              </div>
            </TabsContent>

            <TabsContent value="deliveries" className="mt-4">
              <div className="divide-y">
                {clearedCount !== null && (
                  <div className="pb-4">
                    <Alert>
                      <CheckCircle2Icon className="size-4" />
                      <AlertTitle>Cleared {clearedCount} delivery record{clearedCount === 1 ? '' : 's'}</AlertTitle>
                    </Alert>
                  </div>
                )}
                <DetailPageSection
                  title="Delivery logs"
                  description="Recent delivery attempts and responses."
                  className="px-0"
                  trailing={
                    canManage ? (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => { setOpError(null); setConfirmingClear(true) }}
                        disabled={clearing}
                      >
                        <EraserIcon className="size-4" />
                        Clear history
                      </Button>
                    ) : undefined
                  }
                >
                  <DeliveryLog webhookId={webhook.id} />
                </DetailPageSection>

                {webhook.kind === 'ASYNC' && (
                  <DetailPageSection
                    title="Dead letters"
                    description="Deliveries that exhausted every retry - retry them once the endpoint recovers."
                    className="px-0"
                  >
                    <DeadLetters webhookId={webhook.id} />
                  </DetailPageSection>
                )}
              </div>
            </TabsContent>
          </Tabs>
        </div>
        <PluginSections target="webhook" entity={webhook} mode="edit" />
      </DetailPageBody>

      <NewSecretDialog
        secret={newSecret?.secret ?? null}
        webhookName={newSecret?.name ?? null}
        open={!!newSecret}
        onClose={() => setNewSecret(null)}
      />

      <AlertDialog open={confirmingRotate} onOpenChange={setConfirmingRotate}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Rotate secret?</AlertDialogTitle>
            <AlertDialogDescription>
              This will invalidate the current secret immediately. Any receiver still using the old
              secret will fail signature verification.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={(e) => { e.preventDefault(); onRotateSecret() }}>
              {rotating ? 'Rotating…' : 'Rotate immediately'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmingClear} onOpenChange={setConfirmingClear}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Clear delivery history?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes every recorded delivery attempt for{' '}
              <span className="font-semibold">{webhook.name}</span>. Future deliveries will still be
              logged, but the current history cannot be recovered.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={(e) => { e.preventDefault(); onClearDeliveries() }}
            >
              {clearing ? 'Clearing…' : 'Clear history'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmingDelete} onOpenChange={setConfirmingDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Permanently delete {webhook.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove all delivery logs and configuration. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={(e) => { e.preventDefault(); onDelete() }}
            >
              {deleting ? 'Deleting…' : 'Delete webhook'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}
