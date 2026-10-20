import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { RotateCwIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  ClearWebhookDeadLettersDocument,
  RetryWebhookDeadLetterDocument,
  WebhookDeadLettersDocument,
} from './Webhooks.gql'
import { useRefetchWhenShown, useVisiblePollInterval } from '@/lib/visible-poll'

// DeadLetters lists async deliveries that exhausted the retry ladder Each row can be re-queued for a fresh delivery
// cycle; Clear drops the parked rows for good - Flat divide-y list per dashboard idiom
export function DeadLetters({ webhookId }: { webhookId: string }) {
  const { has } = usePermissions()
  const canManage = has('webhooks.manage')
  const [opError, setOpError] = useState<string | null>(null)

  const { data, loading, error, refetch } = useQuery(WebhookDeadLettersDocument, {
    variables: { webhookId, limit: 50 },
    pollInterval: useVisiblePollInterval(15000),
  })
  useRefetchWhenShown(refetch)
  const [retryItem, { loading: retrying }] = useMutation(RetryWebhookDeadLetterDocument)
  const [clearAll, { loading: clearing }] = useMutation(ClearWebhookDeadLettersDocument)

  const items = (data?.webhookDeadLetters ?? []).filter((i) => i.status === 'DEAD')

  async function onRetry(id: string) {
    setOpError(null)
    try {
      await retryItem({ variables: { id } })
      refetch()
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Retry failed.')
    }
  }

  async function onClear() {
    setOpError(null)
    try {
      await clearAll({ variables: { webhookId } })
      refetch()
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Clear failed.')
    }
  }

  if (loading && !data) {
    return <p className="text-sm text-muted-foreground">Loading dead letters…</p>
  }
  if (error) {
    return <p className="text-sm text-destructive">Couldn't load dead letters: {error.message}</p>
  }
  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No dead letters. Deliveries that fail every retry end up here for inspection and
        redelivery.
      </p>
    )
  }

  return (
    <div className="grid gap-3">
      {opError && <p className="text-sm text-destructive">{opError}</p>}
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">
          {items.length} dead letter{items.length === 1 ? '' : 's'}
        </span>
        {canManage && (
          <Button variant="outline" size="sm" onClick={onClear} disabled={clearing}>
            <Trash2Icon className="size-4" />
            Clear all
          </Button>
        )}
      </div>
      <ul className="divide-y rounded-md border">
        {items.map((item) => (
          <li key={item.id} className="flex flex-wrap items-center gap-3 px-3 py-2 text-sm">
            <Badge variant="outline" className="font-mono text-[10px] uppercase">
              {item.eventType.replace(/\./g, ' ')}
            </Badge>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="min-w-0 flex-1 truncate text-xs text-destructive">
                  {item.lastError ?? 'delivery failed'}
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-md font-mono text-[11px]" side="top">
                {item.lastError ?? 'delivery failed'}
              </TooltipContent>
            </Tooltip>
            <span className="text-xs text-muted-foreground">
              {item.attempts} attempt{item.attempts === 1 ? '' : 's'} · died{' '}
              {new Date(item.updatedAt).toLocaleString()}
            </span>
            {canManage && (
              <Button
                variant="outline"
                size="sm"
                className="h-7 gap-1.5 text-xs"
                onClick={() => onRetry(item.id)}
                disabled={retrying}
              >
                <RotateCwIcon className="size-3" />
                Retry
              </Button>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}
