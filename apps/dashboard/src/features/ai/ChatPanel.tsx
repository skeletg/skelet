import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from 'react'
import { useApolloClient, useMutation, useQuery } from '@apollo/client/react'
import { useLocation } from 'react-router-dom'
import { ArrowUpIcon, HistoryIcon, MessageSquareIcon, SquarePenIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'
import {
  AiChatStatusDocument,
  DeleteChatConversationDocument,
  RunChatActionDocument,
  SaveChatConversationDocument,
  SignalsChatDocument,
  SignalsInsightsDocument,
  ChatConversationDocument,
  ChatConversationsDocument,
} from './Ai.gql'
import { MessageContent } from './ChatMessage'

type ChatMessage = { role: 'user' | 'assistant'; content: string }

const STARTERS = [
  'Summarize how the store is doing.',
  'What changed in traffic recently?',
  'Any anomalies or things I should look at?',
  'Which segment looks most valuable?',
]

// Gradient (blue → purple → pink) used for the greeting text and the send button

// Docked-panel width (px), user-adjustable via the left-edge drag handle and persisted across sessions - The aside
// opens to this width; the card inside is 1rem narrower (the m-2 gutter)
const MIN_W = 320
const MAX_W = 720
const DEFAULT_W = 400 // 25rem - the previous fixed width
const WIDTH_KEY = 'chat:panelWidth'

function loadWidth(): number {
  if (typeof window === 'undefined') return DEFAULT_W
  const saved = Number(window.localStorage.getItem(WIDTH_KEY))
  return saved >= MIN_W && saved <= MAX_W ? saved : DEFAULT_W
}

// ---- page awareness ----

// What page the operator is on. entityType/entityId are set on detail routes so the assistant can look the record up
// (product/order/customer); label is shown in the header chip and sent to the model as context for "this page"
type PageCtx = { path: string; label: string; entityType?: string; entityId?: string }

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const ENTITY_ROOTS: Record<string, string> = { products: 'product', orders: 'order', customers: 'customer' }

function titleCase(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1).replace(/-/g, ' ')
}

function pageContext(pathname: string): PageCtx {
  const segs = pathname.split('/').filter(Boolean)
  if (segs.length === 0) return { path: pathname, label: 'Dashboard' }
  // /products/:id, /orders/:id, /customers/:id  → entity detail
  const root = ENTITY_ROOTS[segs[0]]
  if (root && segs[1] && UUID_RE.test(segs[1])) {
    return { path: pathname, label: `${titleCase(root)} detail`, entityType: root, entityId: segs[1] }
  }
  // /synapse/customers/:id → behavioural customer page
  if (segs[0] === 'synapse' && segs[1] === 'customers' && segs[2] && UUID_RE.test(segs[2])) {
    return { path: pathname, label: 'Customer (Synapse)', entityType: 'customer', entityId: segs[2] }
  }
  return { path: pathname, label: titleCase(segs[0]) }
}

// AssistantMark marks an assistant turn with the same glyph as the trigger
function AssistantMark({ className }: { className?: string }) {
  return <MessageSquareIcon className={cn('text-muted-foreground', className)} />
}

function useHasSignals(): boolean {
  const { has } = usePermissions()
  return (
    has('lens.view') ||
    has('customers.view') ||
    has('emails.view') ||
    has('experiments.view') ||
    has('reports.view')
  )
}

// ---- open/close state shared by the top-bar trigger and the docked panel ----

type ChatCtx = { open: boolean; toggle: () => void; close: () => void }
const Ctx = createContext<ChatCtx | null>(null)

export function ChatProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const value = useMemo<ChatCtx>(
    () => ({ open, toggle: () => setOpen((o) => !o), close: () => setOpen(false) }),
    [open],
  )
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

function useChatDock(): ChatCtx {
  const c = useContext(Ctx)
  if (!c) throw new Error('useChatDock must be used within ChatProvider')
  return c
}

// ChatTrigger is the top-bar icon (left of the bell) - Shows for any admin
// who can view Signals; toggles the docked panel
export function ChatTrigger() {
  const hasSignals = useHasSignals()
  const { enabled: featureEnabled } = useFeatures()
  const { open, toggle } = useChatDock()
  // AI_ENABLED=false hides the chat entirely - no "enable me" teaser
  if (!hasSignals || !featureEnabled('ai')) return null
  return (
    <Button
      variant={open ? 'secondary' : 'ghost'}
      size="icon"
      aria-label="Chat"
      aria-pressed={open}
      onClick={toggle}
    >
      <MessageSquareIcon className="size-4" />
    </Button>
  )
}

// ChatDock is the docked right-hand card - It's a real layout column - when open it shrinks the main content (not
// an overlay) - Renders nothing when closed or when the admin can't view any Signals
export function ChatDock() {
  const hasSignals = useHasSignals()
  const { enabled: featureEnabled } = useFeatures()
  const { open, close } = useChatDock()

  const { data: statusData } = useQuery(AiChatStatusDocument, { skip: !open })
  const enabled = statusData?.aiStatus.enabled ?? false

  const { channels, selectedChannel } = useConfigScope()
  const [chosenChannel, setChosenChannel] = useState('')
  const effectiveChannel = chosenChannel || selectedChannel?.id || channels[0]?.id || ''

  const [days, setDays] = useState('30')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [send, { loading: sending }] = useMutation(SignalsChatDocument)

  // Proactive insights for the empty state - deterministic (no model call)
  const { data: insightsData } = useQuery(SignalsInsightsDocument, {
    variables: { channelId: effectiveChannel, sinceDays: Number(days) },
    skip: !open || !enabled || !effectiveChannel,
  })
  const insights = insightsData?.signalsInsights ?? []

  // ---- conversation history (persisted per admin) ----
  const client = useApolloClient()
  const [conversationId, setConversationId] = useState<string | null>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [saveConv] = useMutation(SaveChatConversationDocument)
  const [deleteConv] = useMutation(DeleteChatConversationDocument)
  const { data: convData, refetch: refetchConvs } = useQuery(ChatConversationsDocument, {
    variables: { limit: 20 },
    skip: !open || !historyOpen,
  })
  const conversations = convData?.chatConversations ?? []

  // Best-effort autosave after each completed exchange; upserts on conversationId
  async function persist(msgs: ChatMessage[]) {
    if (!enabled) return
    try {
      const res = await saveConv({
        variables: {
          id: conversationId,
          channelId: effectiveChannel || null,
          messages: msgs.map((m) => ({ role: m.role, content: m.content })),
        },
      })
      const id = res.data?.saveChatConversation
      if (id && !conversationId) setConversationId(id)
    } catch {
      /* history save is best-effort - never block the chat */
    }
  }

  async function loadConversation(id: string) {
    try {
      const res = await client.query({
        query: ChatConversationDocument,
        variables: { id },
        fetchPolicy: 'network-only',
      })
      const conv = res.data?.chatConversation
      if (conv) {
        setMessages(conv.messages.map((m) => ({ role: m.role === 'assistant' ? 'assistant' : 'user', content: m.content })))
        setConversationId(conv.id)
      }
    } catch {
      /* ignore load failure */
    }
    setHistoryOpen(false)
    setError(null)
  }

  function newChat() {
    setMessages([])
    setConversationId(null)
    setInput('')
    setError(null)
    setHistoryOpen(false)
  }

  async function removeConversation(id: string) {
    try {
      await deleteConv({ variables: { id } })
    } catch {
      /* ignore */
    }
    if (id === conversationId) newChat()
    void refetchConvs()
  }

  // Run an operator-confirmed action the assistant proposed (server re-checks perms)
  const [runAction] = useMutation(RunChatActionDocument)
  async function onAction(action: string, args: unknown): Promise<{ ok: boolean; message: string }> {
    const res = await runAction({ variables: { action, argsJson: JSON.stringify(args ?? {}) } })
    const r = res.data?.runChatAction
    return { ok: r?.ok ?? false, message: r?.message ?? 'Done.' }
  }

  // Page awareness: track the operator's current route so the assistant can answer
  // about "this page" and look up the record it shows
  const { pathname } = useLocation()
  const page = useMemo(() => pageContext(pathname), [pathname])
  const threadRef = useRef<HTMLDivElement>(null)

  async function ask(text: string) {
    const q = text.trim()
    if (!q || sending || !effectiveChannel || !enabled) return
    setError(null)
    const next: ChatMessage[] = [...messages, { role: 'user', content: q }]
    setMessages(next)
    setInput('')
    try {
      const res = await send({
        variables: {
          channelId: effectiveChannel,
          sinceDays: Number(days),
          page: {
            path: page.path,
            label: page.label,
            entityType: page.entityType ?? null,
            entityId: page.entityId ?? null,
          },
          messages: next.map((m) => ({ role: m.role, content: m.content })),
        },
      })
      const reply = res.data?.signalsChat.reply?.trim()
      if (reply) {
        const full: ChatMessage[] = [...next, { role: 'assistant', content: reply }]
        setMessages(full)
        void persist(full)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    }
  }

  // Keep the newest message in view: scroll the thread to the bottom whenever a message is sent/received (messages
  // change), the "Thinking…" row appears, or the panel is (re)opened - Honours reduced-motion
  useEffect(() => {
    const el = threadRef.current
    if (!el) return
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    el.scrollTo({ top: el.scrollHeight, behavior: reduce ? 'auto' : 'smooth' })
  }, [messages, sending, open])

  // ---- resizable width (desktop) ----
  const [width, setWidth] = useState<number>(loadWidth)
  const [resizing, setResizing] = useState(false)
  useEffect(() => {
    try {
      window.localStorage.setItem(WIDTH_KEY, String(width))
    } catch {
      /* localStorage unavailable (private mode / quota) - width just won't persist */
    }
  }, [width])

  function startResize(e: ReactPointerEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startW = width
    setResizing(true)
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
    // Panel sits on the right, so dragging LEFT (smaller clientX) widens it
    const move = (ev: PointerEvent) => {
      setWidth(Math.min(MAX_W, Math.max(MIN_W, startW + (startX - ev.clientX))))
    }
    const end = () => {
      setResizing(false)
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', end)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', end)
  }

  function nudgeWidth(e: ReactKeyboardEvent) {
    if (e.key === 'ArrowLeft') {
      e.preventDefault()
      setWidth((w) => Math.min(MAX_W, w + 16))
    } else if (e.key === 'ArrowRight') {
      e.preventDefault()
      setWidth((w) => Math.max(MIN_W, w - 16))
    }
  }

  // Stays mounted so it can animate open/close (and keep the conversation) The width animates 0 ↔ 24rem, so opening
  // smoothly pushes the main content narrower; the card itself slides + fades in
  if (!hasSignals || !featureEnabled('ai')) return null

  return (
    <aside
      aria-hidden={!open}
      // --sk-w is the user-set width; the open md width tracks it (mobile stays full-width) - Width transition is OFF
      // while dragging so it follows the pointer instantly, ON otherwise (smooth open/close + keyboard nudges)
      style={{ '--sk-w': `${width}px` } as CSSProperties}
      className={cn(
        'flex shrink-0 justify-end overflow-hidden',
        resizing ? '' : 'transition-[width] duration-300 ease-out motion-reduce:transition-none',
        open ? 'w-full md:w-[var(--sk-w)]' : 'w-0',
      )}
    >
      <div
        className={cn(
          // Fixed-per-frame width + shrink-0 so the flex container can't compress
          // the card while the aside animates its width - the overflow-hidden
          // aside just reveals it (no text re-wrap on open/close) - Width tracks
          // --sk-w minus the constant m-2 gutter. `relative` anchors the handle
          'relative m-2 flex w-[calc(100vw_-_1rem)] shrink-0 flex-col overflow-hidden rounded-xl border bg-card shadow-xl transition-[transform,opacity] duration-300 ease-out motion-reduce:transition-none md:w-[calc(var(--sk-w)_-_1rem)]',
          open ? 'translate-x-0 opacity-100' : 'pointer-events-none translate-x-6 opacity-0',
        )}
      >
        {/* Left-edge drag handle to resize the panel (desktop only) */}
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize chat panel"
          aria-valuenow={width}
          aria-valuemin={MIN_W}
          aria-valuemax={MAX_W}
          tabIndex={0}
          onPointerDown={startResize}
          onKeyDown={nudgeWidth}
          className={cn(
            'absolute inset-y-0 left-0 z-20 hidden w-1.5 cursor-col-resize touch-none outline-none transition-colors md:block',
            'hover:bg-primary/30 focus-visible:bg-primary/40',
            resizing ? 'bg-primary/40' : 'bg-transparent',
          )}
        />
        <div className="flex items-center gap-2 px-4 py-3">
          <div className="flex shrink-0 items-center gap-2">
            <AssistantMark className="size-7" />
            <span className="text-base font-semibold">Chat</span>
          </div>
          {/* Page-awareness chip: the page the assistant will answer about */}
          <span
            className="ml-auto min-w-0 max-w-[11rem] truncate rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground"
            title={`Viewing: ${page.path}`}
          >
            {page.label}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="size-8 shrink-0 rounded-full"
            aria-label="New chat"
            onClick={newChat}
          >
            <SquarePenIcon className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className={cn('size-8 shrink-0 rounded-full', historyOpen && 'bg-muted')}
            aria-label="Chat history"
            aria-pressed={historyOpen}
            onClick={() => setHistoryOpen((o) => !o)}
          >
            <HistoryIcon className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="size-8 shrink-0 rounded-full" aria-label="Close" onClick={close}>
            <XIcon className="size-4" />
          </Button>
        </div>

        <div className="flex items-center gap-2 px-4 pb-2">
          <Select value={effectiveChannel} onValueChange={setChosenChannel}>
            <SelectTrigger className="h-7 flex-1 rounded-full border-0 bg-muted text-xs"><SelectValue placeholder="Channel" /></SelectTrigger>
            <SelectContent>
              {channels.map((c) => (
                <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={days} onValueChange={setDays}>
            <SelectTrigger className="h-7 w-28 rounded-full border-0 bg-muted text-xs"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="7">Last 7 days</SelectItem>
              <SelectItem value="30">Last 30 days</SelectItem>
              <SelectItem value="90">Last 90 days</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div ref={threadRef} className="scrollbar-accent flex-1 min-h-0 space-y-5 overflow-y-auto px-4 py-4">
          {historyOpen ? (
            <div className="space-y-1">
              <div className="px-1 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Recent chats
              </div>
              {conversations.length === 0 ? (
                <p className="px-1 py-4 text-sm text-muted-foreground">No saved chats yet.</p>
              ) : (
                conversations.map((c) => (
                  <div key={c.id} className="group flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-muted/60">
                    <button
                      type="button"
                      className="min-w-0 flex-1 truncate text-left text-sm"
                      onClick={() => void loadConversation(c.id)}
                    >
                      {c.title || 'Untitled chat'}
                    </button>
                    <button
                      type="button"
                      aria-label="Delete chat"
                      className="shrink-0 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
                      onClick={() => void removeConversation(c.id)}
                    >
                      <XIcon className="size-3.5" />
                    </button>
                  </div>
                ))
              )}
            </div>
          ) : !enabled ? (
            <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              Chat isn't enabled yet. Turn it on and add a provider + API key under{' '}
              <span className="font-medium text-foreground">Settings → AI</span>, then reopen this panel.
            </div>
          ) : messages.length === 0 ? (
            <div className="flex h-full flex-col">
              <div className="mb-5 mt-2">
                <h2 className="text-2xl font-semibold leading-tight">Hello there</h2>
                <p className="mt-1 text-sm text-muted-foreground">How can I help you analyze your Signals today?</p>
              </div>
              {insights.length > 0 ? (
                <div className="mb-5 space-y-2">
                  <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Here's what changed</div>
                  {insights.map((it, idx) => (
                    <button
                      key={idx}
                      type="button"
                      onClick={() => void ask(`Tell me more about: ${it.title}`)}
                      className={cn(
                        'flex w-full items-start gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors hover:bg-muted/60',
                        it.level === 'warn'
                          ? 'border-amber-500/40'
                          : it.level === 'good'
                            ? 'border-emerald-500/40'
                            : '',
                      )}
                    >
                      <span
                        className={cn(
                          'mt-1.5 size-2 shrink-0 rounded-full',
                          it.level === 'warn'
                            ? 'bg-amber-500'
                            : it.level === 'good'
                              ? 'bg-emerald-500'
                              : 'bg-muted-foreground/50',
                        )}
                      />
                      <span className="min-w-0">
                        <span className="font-medium">{it.title}</span>{' '}
                        <span className="text-muted-foreground">- {it.detail}</span>
                      </span>
                    </button>
                  ))}
                </div>
              ) : null}
              <div className="grid gap-2">
                {STARTERS.map((s) => (
                  <button
                    key={s}
                    type="button"
                    className="group flex items-center justify-between gap-2 rounded-md border bg-muted/40 px-4 py-3 text-left text-sm transition-colors hover:bg-muted"
                    onClick={() => void ask(s)}
                  >
                    <span>{s}</span>
                    <ArrowUpIcon className="size-4 shrink-0 rotate-45 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                  </button>
                ))}
              </div>
            </div>
          ) : (
            messages.map((m, i) =>
              m.role === 'user' ? (
                <div
                  key={i}
                  className="flex justify-end"
                >
                  <div className="max-w-[85%] whitespace-pre-wrap rounded-3xl bg-muted px-4 py-2.5 text-sm">
                    {m.content}
                  </div>
                </div>
              ) : (
                <div
                  key={i}
                  className="flex gap-3"
                >
                  <AssistantMark className="mt-0.5 size-7 shrink-0" />
                  <MessageContent
                    content={m.content}
                    onAsk={i === messages.length - 1 && !sending ? (q) => void ask(q) : undefined}
                    onAction={i === messages.length - 1 && !sending ? onAction : undefined}
                  />
                </div>
              ),
            )
          )}
          {sending ? (
            <div className="flex items-center gap-3">
              <AssistantMark className="size-7 shrink-0" />
              <span className="animate-pulse text-sm text-muted-foreground">Thinking…</span>
            </div>
          ) : null}
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="p-3">
          <form
            className="flex items-center gap-1 rounded-full border bg-background py-1 pl-4 pr-1 shadow-sm transition focus-within:ring-2 focus-within:ring-ring/40"
            onSubmit={(e) => {
              e.preventDefault()
              void ask(input)
            }}
          >
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={!enabled ? 'AI not enabled' : effectiveChannel ? 'Ask a question…' : 'No channel available'}
              disabled={sending || !effectiveChannel || !enabled}
              className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground disabled:opacity-50"
            />
            <Button
              type="submit"
              size="icon"
              disabled={sending || !input.trim() || !effectiveChannel || !enabled}
              className="size-9 shrink-0 rounded-full"
            >
              <ArrowUpIcon className="size-4" />
            </Button>
          </form>
          <p className="mt-2 text-center text-[11px] text-muted-foreground">Chat can make mistakes - double-check important numbers.</p>
        </div>
      </div>
    </aside>
  )
}
