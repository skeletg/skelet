import { useMutation, useQuery } from '@apollo/client/react'
import { useNavigate } from 'react-router-dom'
import {
  MarkNotificationReadDocument,
  NotificationUnreadCountDocument,
  NotificationsDocument,
} from '@/features/notifications/Notifications.gql'
import { CATEGORY_LABEL, INBOX_PEEK } from '@/features/notifications/preferences'
import { WidgetSection } from './primitives'

const ROWS = 5

const SEVERITY_DOT: Record<string, string> = {
  critical: 'bg-red-500',
  warning: 'bg-amber-500',
  info: 'bg-sky-500',
}

function ago(iso: string): string {
  const mins = Math.round((Date.now() - new Date(iso).getTime()) / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  if (mins < 1440) return `${Math.round(mins / 60)}h ago`
  return `${Math.round(mins / 1440)}d ago`
}

// The bell's dropdown as a section - same rows, but sitting on the page you already have open, and
// muted categories are absent because the server filters them out of this query too
export function InboxWidget() {
  const navigate = useNavigate()
  // cache-first, deliberately: the bell asks for the same rows and keeps them fresh through the
  // live subscription, so the section reads what the bell already fetched instead of asking again
  const { data, loading, error, refetch } = useQuery(NotificationsDocument, {
    variables: { limit: INBOX_PEEK },
    fetchPolicy: 'cache-first',
  })
  const countQuery = useQuery(NotificationUnreadCountDocument, { fetchPolicy: 'cache-first' })
  const [markRead] = useMutation(MarkNotificationReadDocument)
  const items = (data?.notifications ?? []).slice(0, ROWS)
  const unread = countQuery.data?.notificationUnreadCount ?? 0

  const open = async (id: string, read: boolean, actionUrl: string) => {
    if (!read) {
      await markRead({ variables: { id } }).catch(() => {})
      void refetch()
      void countQuery.refetch()
    }
    if (actionUrl) navigate(actionUrl)
  }

  return (
    <WidgetSection
      title="Inbox"
      description="Store-ops notices, newest first."
      to="/inbox"
      loading={loading && items.length === 0}
      error={error ?? null}
      empty={!loading && items.length === 0}
      trailing={
        unread > 0 ? (
          <span className="text-xs text-muted-foreground">{unread.toLocaleString()} unread</span>
        ) : null
      }
    >
      <div className="divide-y rounded-md border">
        {items.map((n) => (
          <button
            key={n.id}
            type="button"
            onClick={() => void open(n.id, n.read, n.actionUrl)}
            className="flex w-full items-start gap-3 px-3 py-2 text-left transition hover:bg-muted/40"
          >
            <span className={`mt-1.5 size-2 shrink-0 rounded-full ${SEVERITY_DOT[n.severity] ?? 'bg-muted-foreground'}`} />
            <span className="min-w-0 flex-1">
              <span className={`block text-sm ${n.read ? 'text-muted-foreground' : 'font-medium'}`}>
                {n.title}
              </span>
              <span className="line-clamp-2 block text-xs text-muted-foreground">{n.body}</span>
            </span>
            <span className="shrink-0 text-right text-xs text-muted-foreground">
              <span className="block">{CATEGORY_LABEL[n.category] ?? n.category}</span>
              <span className="block">{ago(n.createdAt)}</span>
            </span>
          </button>
        ))}
      </div>
    </WidgetSection>
  )
}
