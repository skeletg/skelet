import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@apollo/client/react'
import {
  ChevronLeftIcon,
  ExternalLinkIcon,
  LoaderCircleIcon,
  MonitorIcon,
  MoonIcon,
  SmartphoneIcon,
  SunIcon,
  TriangleAlertIcon,
  XIcon,
} from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import type { EmailBlock, EmailStyles } from '@/features/email/blocks'
import { emailFacts, formatBytes, GMAIL_CLIP_BYTES, previewDocument } from '@/features/email/previewDoc'
import { PreviewCustomersDocument, RenderLibraryPreviewDocument } from './Emails.gql'

type Device = 'desktop' | 'mobile'
type View = 'email' | 'text'

const SAMPLE = '__sample__'

function Segmented<T extends string>({ label, value, onChange, options }: { label: string; value: T; onChange: (v: T) => void; options: Array<{ v: T; label: string; icon?: ReactNode }> }) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex items-center gap-0.5 rounded-md border bg-background p-0.5">
      {options.map((o) => (
        <button
          key={o.v}
          type="button"
          role="radio"
          aria-checked={value === o.v}
          onClick={() => onChange(o.v)}
          className={cn(
            'inline-flex h-7 items-center gap-1.5 rounded px-2.5 text-xs transition-colors',
            value === o.v ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {o.icon}
          {o.label}
        </button>
      ))}
    </div>
  )
}

export function EmailFrame({ html, dark, className }: { html: string; dark: boolean; className?: string }) {
  const ref = useRef<HTMLIFrameElement>(null)
  const observer = useRef<ResizeObserver | null>(null)
  const [height, setHeight] = useState(480)
  const doc = useMemo(() => previewDocument(html, { dark }), [html, dark])
  useEffect(() => () => observer.current?.disconnect(), [])
  function measure() {
    const d = ref.current?.contentDocument
    if (!d?.documentElement) return
    const fit = () => setHeight(Math.max(120, Math.ceil(d.documentElement.getBoundingClientRect().height)))
    fit()
    observer.current?.disconnect()
    if (typeof ResizeObserver !== 'undefined' && d.body) {
      observer.current = new ResizeObserver(fit)
      observer.current.observe(d.body)
    }
  }
  return (
    <iframe
      ref={ref}
      title="Email preview"
      // No scripts ever run in here; same-origin only lets the frame be measured, popups let a link be checked
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      srcDoc={doc}
      onLoad={measure}
      style={{ height }}
      className={cn('block w-full border-0 bg-white', className)}
    />
  )
}

function Avatar({ name }: { name: string }) {
  return (
    <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-full bg-muted text-base font-medium text-foreground">
      {(name.trim().charAt(0) || '?').toUpperCase()}
    </span>
  )
}

export type Rendered = { subject: string; html: string; text: string; preheader: string; fromName: string; fromAddress: string }

export function InboxRow({ r, compact }: { r: Rendered; compact?: boolean }) {
  return (
    <div className="flex items-start gap-3 rounded-lg border bg-background px-4 py-3">
      <Avatar name={r.fromName} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <span className="truncate text-sm font-semibold text-foreground">{r.fromName || r.fromAddress}</span>
          <span className="shrink-0 text-xs text-muted-foreground">9:41 AM</span>
        </div>
        {compact ? (
          <>
            <p className="truncate text-sm font-semibold text-foreground">{r.subject || '(no subject)'}</p>
            <p className="line-clamp-2 text-sm text-muted-foreground">{r.preheader || r.text}</p>
          </>
        ) : (
          <p className="truncate text-sm">
            <span className="font-semibold text-foreground">{r.subject || '(no subject)'}</span>
            <span className="text-muted-foreground"> - {r.preheader || r.text}</span>
          </p>
        )}
      </div>
    </div>
  )
}

function MessageHead({ r, to, compact }: { r: Rendered; to: string; compact?: boolean }) {
  return (
    <div className={cn('border-b', compact ? 'px-4 py-3' : 'px-6 py-5')}>
      <h3 className={cn('font-normal leading-snug text-foreground', compact ? 'text-lg' : 'text-[1.375rem]')}>{r.subject || '(no subject)'}</h3>
      <div className="mt-3 flex items-center gap-3">
        <Avatar name={r.fromName} />
        <div className="min-w-0 flex-1 text-sm leading-tight">
          <p className="truncate">
            <span className="font-medium text-foreground">{r.fromName}</span>{' '}
            <span className="text-muted-foreground">&lt;{r.fromAddress}&gt;</span>
          </p>
          <p className="truncate text-xs text-muted-foreground">to {to}</p>
        </div>
      </div>
    </div>
  )
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-1">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="text-sm">{children}</div>
    </div>
  )
}

function Warn({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-start gap-1.5 text-xs text-warning">
      <TriangleAlertIcon className="mt-px size-3.5 shrink-0" />
      <span>{children}</span>
    </p>
  )
}

function Details({ r }: { r: Rendered }) {
  const facts = useMemo(() => emailFacts(r.html), [r.html])
  const broken = facts.links.filter((l) => !l.ok).length
  return (
    <div className="grid grid-cols-1 gap-5">
      <Fact label="From">
        <p className="break-words">{r.fromName}</p>
        <p className="break-all text-xs text-muted-foreground">{r.fromAddress}</p>
      </Fact>
      <Fact label={`Subject · ${r.subject.length} characters`}>
        <p className="break-words">{r.subject || <span className="text-muted-foreground">(no subject)</span>}</p>
        {!r.subject.trim() ? <Warn>An email without a subject reads as spam.</Warn> : null}
        {r.subject.length > 60 ? <Warn>Phones show about the first 40 characters.</Warn> : null}
      </Fact>
      <Fact label="Preheader">
        {r.preheader ? <p className="break-words">{r.preheader}</p> : <Warn>None - inboxes show the email's first words after the subject instead.</Warn>}
      </Fact>
      <Fact label="Size">
        <p>{formatBytes(facts.bytes)}</p>
        {facts.bytes > GMAIL_CLIP_BYTES ? (
          <Warn>Gmail clips emails over 102 KB behind a "View entire message" link.</Warn>
        ) : (
          <p className="text-xs text-muted-foreground">Under Gmail's 102 KB clipping limit.</p>
        )}
      </Fact>
      <Fact label={`Images · ${facts.images}`}>
        {facts.imagesWithoutAlt > 0 ? (
          <Warn>
            {facts.imagesWithoutAlt} without alt text - it's what shows while images are blocked.
          </Warn>
        ) : (
          <p className="text-xs text-muted-foreground">{facts.images ? 'All have alt text.' : 'None.'}</p>
        )}
      </Fact>
      <Fact label={`Links · ${facts.links.length}${broken ? `, ${broken} to check` : ''}`}>
        {facts.links.length === 0 ? (
          <p className="text-xs text-muted-foreground">None.</p>
        ) : (
          <ul className="grid grid-cols-1 gap-1.5">
            {facts.links.map((l, i) => (
              <li key={i} className="grid min-w-0 grid-cols-1 gap-0.5 rounded-md border px-2.5 py-1.5">
                <span className="truncate text-xs font-medium">{l.text}</span>
                {l.ok ? (
                  <a href={l.href} target="_blank" rel="noreferrer noopener" className="flex items-center gap-1 truncate text-xs text-muted-foreground hover:text-foreground">
                    <span className="truncate">{l.href}</span>
                    <ExternalLinkIcon className="size-3 shrink-0" />
                  </a>
                ) : (
                  <Warn>{l.href ? `Goes nowhere real: ${l.href}` : 'No address'}</Warn>
                )}
              </li>
            ))}
          </ul>
        )}
      </Fact>
    </div>
  )
}

export function PreviewDialog({
  open,
  onOpenChange,
  channelId,
  subject,
  preheader,
  blocks,
  styles,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  channelId: string
  subject: string
  preheader: string
  blocks: EmailBlock[]
  styles: EmailStyles
}) {
  const [device, setDevice] = useState<Device>('desktop')
  const [view, setView] = useState<View>('email')
  const [dark, setDark] = useState(false)
  const [customerId, setCustomerId] = useState<string | null>(null)
  const [theme] = useState(() => (typeof document === 'undefined' ? undefined : document.documentElement.dataset.theme))

  const { data: custData } = useQuery(PreviewCustomersDocument, { variables: { channelId }, skip: !open, fetchPolicy: 'cache-first' })
  const customers = custData?.customersPaged.items ?? []
  const customer = customers.find((c) => c.id === customerId)

  const { data, previousData, loading, error } = useQuery(RenderLibraryPreviewDocument, {
    variables: { channelId, customerId, subject, preheader, blocks, styles },
    skip: !open,
    fetchPolicy: 'no-cache',
  })
  const p = (data ?? previousData)?.renderLibraryPreview
  const r: Rendered | null = p
    ? { subject: p.subject, html: p.html, text: p.text ?? '', preheader: p.preheader ?? '', fromName: p.fromName ?? '', fromAddress: p.fromAddress ?? '' }
    : null
  const to = customer ? customer.email : 'me'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="flex h-[92vh] w-[min(1280px,96vw)] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none">
        <div className="flex flex-wrap items-center gap-2 border-b px-4 py-2.5">
          <DialogTitle className="mr-2 text-base">Preview</DialogTitle>
          <DialogDescription className="sr-only">The email as a recipient gets it, rendered by the server.</DialogDescription>
          <Segmented
            label="View"
            value={view}
            onChange={setView}
            options={[
              { v: 'email', label: 'Email' },
              { v: 'text', label: 'Plain text' },
            ]}
          />
          <Segmented
            label="Width"
            value={device}
            onChange={setDevice}
            options={[
              { v: 'desktop', label: 'Desktop', icon: <MonitorIcon className="size-3.5" /> },
              { v: 'mobile', label: 'Mobile', icon: <SmartphoneIcon className="size-3.5" /> },
            ]}
          />
          {view === 'email' ? (
            <Segmented
              label="Colour scheme"
              value={dark ? 'dark' : 'light'}
              onChange={(v) => setDark(v === 'dark')}
              options={[
                { v: 'light', label: 'Light', icon: <SunIcon className="size-3.5" /> },
                { v: 'dark', label: 'Dark', icon: <MoonIcon className="size-3.5" /> },
              ]}
            />
          ) : null}
          <Select value={customerId ?? SAMPLE} onValueChange={(v) => setCustomerId(v === SAMPLE ? null : v)}>
            <SelectTrigger aria-label="Preview as" className="h-8 w-60 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={SAMPLE}>Sample customer</SelectItem>
              {customers.map((c) => (
                <SelectItem key={c.id} value={c.id}>
                  {[c.firstName, c.lastName].filter(Boolean).join(' ') || c.email}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <span className="ml-auto flex items-center gap-2">
            {loading ? (
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <LoaderCircleIcon className="size-3.5 animate-spin" /> Rendering…
              </span>
            ) : null}
            <DialogClose asChild>
              <Button type="button" variant="ghost" size="icon" className="size-8" aria-label="Close preview">
                <XIcon className="size-4" />
              </Button>
            </DialogClose>
          </span>
        </div>

        <div className="flex min-h-0 flex-1">
          <div className={cn('min-w-0 flex-1 overflow-y-auto bg-muted/40 p-4 2xl:p-6', dark && view === 'email' && 'dark bg-background')} data-theme={dark ? theme : undefined}>
            {error ? (
              <Alert variant="destructive" className="mx-auto max-w-xl">
                <AlertDescription>The preview didn't render: {error.message}</AlertDescription>
              </Alert>
            ) : !r ? (
              <div className="grid h-full place-items-center text-sm text-muted-foreground">
                <LoaderCircleIcon className="size-5 animate-spin" />
              </div>
            ) : view === 'text' ? (
              <div className={cn('mx-auto rounded-lg border bg-background', device === 'mobile' ? 'max-w-[390px]' : 'max-w-3xl')}>
                <p className="border-b px-4 py-2.5 text-xs text-muted-foreground">Sent alongside the HTML, for inboxes that only show text.</p>
                <pre className="whitespace-pre-wrap break-words px-4 py-4 font-mono text-[13px] leading-relaxed text-foreground">{r.text || '(empty)'}</pre>
              </div>
            ) : device === 'desktop' ? (
              <div className="mx-auto grid max-w-3xl grid-cols-1 gap-5 text-foreground">
                <section aria-label="In the inbox" className="grid min-w-0 grid-cols-1 gap-1.5">
                  <p className="text-xs font-medium text-muted-foreground">In the inbox</p>
                  <InboxRow r={r} />
                </section>
                <section aria-label="Opened" className="grid min-w-0 grid-cols-1 gap-1.5">
                  <p className="text-xs font-medium text-muted-foreground">Opened</p>
                  <div className="overflow-hidden rounded-lg border bg-background">
                    <MessageHead r={r} to={to} />
                    <EmailFrame html={r.html} dark={dark} />
                  </div>
                </section>
              </div>
            ) : (
              <div className="flex flex-wrap items-start justify-center gap-8 text-foreground">
                <section aria-label="In the inbox" className="grid w-[360px] min-w-0 grid-cols-1 gap-1.5">
                  <p className="text-xs font-medium text-muted-foreground">In the inbox</p>
                  <InboxRow r={r} compact />
                </section>
                <section aria-label="Opened" className="grid gap-1.5">
                  <p className="text-xs font-medium text-muted-foreground">Opened</p>
                  <div className="h-[min(740px,calc(92vh-8.5rem))] w-[390px] overflow-hidden rounded-[2.75rem] border-[10px] border-foreground/85 bg-background shadow-xl">
                    <div className="flex h-full flex-col">
                      <div className="flex shrink-0 items-center justify-between px-6 pb-1 pt-3 text-xs font-semibold">
                        <span>9:41</span>
                        <span className="h-1.5 w-16 rounded-full bg-foreground/80" aria-hidden />
                        <span aria-hidden>●●●</span>
                      </div>
                      <div className="flex shrink-0 items-center gap-1 px-3 py-2 text-sm text-muted-foreground">
                        <ChevronLeftIcon className="size-4" /> Inbox
                      </div>
                      <div className="min-h-0 flex-1 overflow-y-auto">
                        <MessageHead r={r} to={to} compact />
                        <EmailFrame html={r.html} dark={dark} />
                      </div>
                    </div>
                  </div>
                </section>
              </div>
            )}
          </div>
          {r ? (
            <aside className="hidden w-72 shrink-0 overflow-y-auto border-l p-4 lg:block 2xl:w-80">
              <Details r={r} />
            </aside>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
