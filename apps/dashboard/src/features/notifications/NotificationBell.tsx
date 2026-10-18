import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useSubscription } from '@apollo/client/react'
import { BellIcon } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  MarkAllNotificationsReadDocument,
  MarkNotificationReadDocument,
  NotificationAddedDocument,
  NotificationUnreadCountDocument,
  NotificationsDocument,
  type NotificationAddedSubscription,
  type NotificationsQuery,
} from './Notifications.gql'
import { INBOX_PEEK, severityAtLeast, useToastMinSeverity } from './preferences'
import { timeAgo } from '@/lib/format'

type Row = NotificationsQuery['notifications'][number]
type PushRow = NotificationAddedSubscription['notificationAdded']

const SEVERITY_DOT: Record<string, string> = {
  critical: 'bg-destructive',
  warning: 'bg-warning',
  info: 'bg-info',
}

// NotificationToast is the live-push popup: the SAME row design as the bell dropdown (severity dot + title + time + two-line body) lifted
// onto the popover surface, so a toast reads as "one of those, arriving now" - The whole card is the action, exactly like a dropdown row
function NotificationToast({ n, onOpen }: { n: PushRow; onOpen: () => void }) {
  return (
    <button
      onClick={onOpen}
      className="flex w-96 cursor-pointer items-start gap-2.5 rounded-md border bg-popover px-3 py-2.5 text-left text-popover-foreground shadow-md"
    >
      <span className={`mt-1.5 size-2 shrink-0 rounded-full ${SEVERITY_DOT[n.severity] ?? 'bg-muted-foreground'}`} />
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline justify-between gap-2">
          <span className="truncate text-sm font-medium">{n.title}</span>
          <span className="shrink-0 text-[11px] text-muted-foreground">just now</span>
        </span>
        {n.body && <span className="mt-0.5 line-clamp-2 block text-xs text-muted-foreground">{n.body}</span>}
      </span>
    </button>
  )
}

// NotificationBell is the header inbox: a bell with an unread badge, a
// dropdown of the most recent items, and a link to the full /inbox page
// Freshness comes from the notificationAdded subscription (instant push +
// toast); the poll stays as a slow correctness backfill for anything a
// socket drop missed
export function NotificationBell() {
  const navigate = useNavigate()
  // cache-first, not cache-and-network: on a phone this lives in the sidebar, which is a Sheet
  // that unmounts when closed, so cache-and-network fired both queries on EVERY drawer open.
  // Freshness already comes from the subscription and the 5-minute poll, and the mount reads
  // whatever they last wrote
  const { data: countData, refetch: refetchCount } = useQuery(NotificationUnreadCountDocument, {
    pollInterval: 300000, // 5min backfill; the subscription is the fast path
    fetchPolicy: 'cache-first',
  })
  const { data, refetch: refetchList } = useQuery(NotificationsDocument, {
    variables: { limit: INBOX_PEEK },
    fetchPolicy: 'cache-first',
  })
  const [markRead] = useMutation(MarkNotificationReadDocument)
  const [markAll] = useMutation(MarkAllNotificationsReadDocument)
  // muted categories never even arrive - the server filters the subscription too - so this only
  // decides how loud the ones that DO arrive are
  const { min: toastMin } = useToastMinSeverity()

  const unread = countData?.notificationUnreadCount ?? 0
  const items = data?.notifications ?? []

  const refresh = () => {
    void refetchCount()
    void refetchList()
  }

  // Live push: refresh badge + list and pop a toast the moment a visible
  // notification is created anywhere in the backend fleet - The toast is a
  // custom render sharing the dropdown row's design (severity dot + title +
  // body on the popover surface); clicking it behaves like clicking the row:
  // mark read, jump to the action URL
  useSubscription(NotificationAddedDocument, {
    onData: ({ data: sub }) => {
      const n = sub.data?.notificationAdded
      if (!n) return
      refresh()
      if (!severityAtLeast(n.severity, toastMin)) return // still in the inbox, just not in your face
      toast.custom((t) => (
        <NotificationToast
          n={n}
          onOpen={() => {
            toast.dismiss(t)
            void markRead({ variables: { id: n.id } }).catch(() => {})
            refresh()
            if (n.actionUrl) navigate(n.actionUrl)
          }}
        />
      ))
    },
    // A failed socket must never break the header - the poll still covers us But in dev, surface the reason: a
    // swallowed error here is indistinguishable from "no notifications happened"
    onError: (err) => {
      if (import.meta.env.DEV) console.warn('[notifications] subscription error', err)
    },
  })

  const open = async (n: Row) => {
    if (!n.read) {
      await markRead({ variables: { id: n.id } }).catch(() => {})
      refresh()
    }
    if (n.actionUrl) navigate(n.actionUrl)
  }

  const onMarkAll = async () => {
    await markAll().catch(() => {})
    refresh()
  }

  return (
    <DropdownMenu onOpenChange={(o) => o && refresh()}>
      <DropdownMenuTrigger asChild>
        <Button data-tour="bell" variant="ghost" size="icon" aria-label="Notifications" className="relative">
          <BellIcon className="size-4" />
          {unread > 0 && (
            <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold leading-none text-destructive-foreground">
              {unread > 9 ? '9+' : unread}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-96 p-0">
        <div className="flex items-center justify-between px-3 py-2">
          <span className="text-sm font-medium">Notifications</span>
          {unread > 0 && (
            <button onClick={onMarkAll} className="text-xs text-muted-foreground hover:text-foreground hover:underline">
              Mark all read
            </button>
          )}
        </div>
        <div className="max-h-96 overflow-y-auto border-t">
          {items.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-muted-foreground">You&apos;re all caught up 🎉</p>
          ) : (
            items.map((n) => (
              <DropdownMenuItem
                key={n.id}
                onClick={() => void open(n)}
                className="flex cursor-pointer items-start gap-2.5 px-3 py-2.5"
              >
                <span className={`mt-1.5 size-2 shrink-0 rounded-full ${SEVERITY_DOT[n.severity] ?? 'bg-muted-foreground'}`} />
                <span className="min-w-0 flex-1">
                  <span className="flex items-baseline justify-between gap-2">
                    <span className={`truncate text-sm ${n.read ? 'font-normal text-muted-foreground' : 'font-medium'}`}>{n.title}</span>
                    <span className="shrink-0 text-[11px] text-muted-foreground">{timeAgo(n.createdAt)}</span>
                  </span>
                  {n.body && <span className="mt-0.5 line-clamp-2 block text-xs text-muted-foreground">{n.body}</span>}
                </span>
              </DropdownMenuItem>
            ))
          )}
        </div>
        <DropdownMenuItem
          onClick={() => navigate('/inbox')}
          className="cursor-pointer justify-center border-t py-2 text-sm font-medium text-primary"
        >
          View all
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
