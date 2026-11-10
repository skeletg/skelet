import { useNavLabel } from '@/nav/nav-i18n'
import { Trans, useLingui } from '@lingui/react/macro'
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { Command as CommandPrimitive, useCommandState } from 'cmdk'
import { AlertCircleIcon, CornerDownLeftIcon, FileIcon, Loader2Icon, SearchIcon, SearchXIcon, SettingsIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Command, CommandDialog, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { useVisiblePages } from '@/nav/use-visible-pages'
import { useConfigScope } from '@/lib/config-scope'
import { useRecentPaths } from '@/lib/recent-pages'
import { useAuth } from '@/shared/auth/auth'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { AdminSearchDocument, type AdminSearchQuery } from './AdminSearch.gql'

// SYNC: apps/backend/core/internal/adminsearch MinTermRunes - shorter terms find nothing there, so they aren't sent
const MIN_RECORD_TERM = 3
// How long typing has to pause before records are looked up
const RECORD_SEARCH_DEBOUNCE_MS = 200

type SearchResult = AdminSearchQuery['adminSearch']

// Each kind of record the palette lists: where its rows open, which list page lends them its icon, and how a row reads
const RECORD_KINDS = [
  {
    kind: 'orders',
    list: '/orders',
    rows: (r: SearchResult) =>
      r.orders.map((o) => ({ key: `order-${o.id}`, path: `/orders/${o.id}`, title: o.number, context: o.name || o.email })),
  },
  {
    kind: 'customers',
    list: '/customers',
    rows: (r: SearchResult) =>
      r.customers.map((c) => ({ key: `customer-${c.id}`, path: `/customers/${c.id}`, title: c.name || c.email, context: c.name ? c.email : undefined })),
  },
  {
    kind: 'products',
    list: '/products',
    rows: (r: SearchResult) =>
      r.products.map((p) => ({ key: `product-${p.id}`, path: `/products/${p.id}`, title: p.name || p.sku, context: p.name ? p.sku : undefined })),
  },
] as const

// Global admin search (⌘K / Ctrl+K, or the header button): one palette over orders, customers and products, every
// sidebar page and every settings section/group, so "where does X live" is a keystroke instead of a menu hunt
// Visibility follows the sidebar's own rules - permission-gated and feature-gated entries a user
// can't see in the nav don't surface in search either
export function AdminSearch() {
  const { t } = useLingui()
  const navLabel = useNavLabel()
  const [open, setOpenState] = useState(false)
  const [query, setQuery] = useState('')
  // However it closes - Esc, a click outside, ⌘K again or picking a row - the next opening starts empty
  const setOpen = useCallback((next: boolean) => {
    setOpenState(next)
    if (!next) setQuery('')
  }, [])
  const navigate = useNavigate()
  const { scopeInput } = useConfigScope()
  const { auth } = useAuth()
  const recentPaths = useRecentPaths(auth?.admin.id ?? '')

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen(!open)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, setOpen])

  // the settings registry is only worth fetching once the palette opens - it's also
  // scope-shaped, so the results match what the settings page would actually show
  const sectionsQuery = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    skip: !open,
  })

  const { pages } = useVisiblePages()

  // Records as well as pages - the server picks which kinds and channels this admin may search
  const typed = query.trim()
  const term = useDebounced(typed, RECORD_SEARCH_DEBOUNCE_MS)
  const searching = open && term.length >= MIN_RECORD_TERM
  const recordsQuery = useQuery(AdminSearchDocument, { variables: { term }, skip: !searching })
  // Only an answer to exactly what is typed is shown - rows for an earlier term, still matching through their keyword,
  // would put another record under the cursor for an Enter pressed before the new answer lands
  const settled = searching && term === typed
  const lookingUp = typed.length >= MIN_RECORD_TERM && (!settled || recordsQuery.loading)
  const recordError = settled ? recordsQuery.error : undefined
  const records = useMemo(() => {
    const data = settled ? recordsQuery.data?.adminSearch : undefined
    if (!data) return []
    const icons = new Map(pages.map((p) => [p.path, p.icon]))
    const headings = { orders: t`Orders`, customers: t`Customers`, products: t`Products` }
    return RECORD_KINDS.map((k) => ({ heading: headings[k.kind], icon: icons.get(k.list), rows: k.rows(data) })).filter((g) => g.rows.length > 0)
  }, [settled, recordsQuery.data, pages, t])

  const settingsEntries = useMemo(() => {
    const out: Array<{ path: string; title: string; context: string }> = []
    for (const section of sectionsQuery.data?.configSections ?? []) {
      if (section.slug === 'search') continue // has its own dedicated page
      const withFields = section.groups.filter((g) => g.fields.length > 0)
      if (withFields.length === 0) continue
      out.push({ path: `/settings/${section.slug}`, title: section.label, context: 'Settings' })
      for (const group of withFields) {
        out.push({
          path: `/settings/${section.slug}/${group.slug}`,
          title: group.label,
          context: `Settings › ${section.label}`,
        })
      }
    }
    return out
  }, [sectionsQuery.data])

  // resolved through the permission-filtered pages list, so a revoked page drops out of recents
  const recents = useMemo(() => {
    const byPath = new Map(pages.map((p) => [p.path, p]))
    return recentPaths.map((p) => byPath.get(p)).filter((p) => p !== undefined)
  }, [recentPaths, pages])

  const go = (path: string) => {
    setOpen(false)
    setQuery('')
    navigate(path)
  }

  return (
    <>
      {/* Icon-only under sm: the placeholder and the shortcut chip are dead weight on a phone
          (no ⌘K to press) and the full-width pill crowded the tab strip out of the header */}
      <Button
        data-tour="search"
        variant="outline"
        onClick={() => setOpen(true)}
        aria-label="Search"
        className="size-8 justify-center gap-2 rounded-full p-0 font-normal text-muted-foreground sm:h-8 sm:w-44 sm:justify-start sm:px-3.5"
      >
        <SearchIcon className="size-4 shrink-0" />
        <span className="hidden flex-1 truncate text-left text-xs sm:inline">Search…</span>
        <kbd className="pointer-events-none hidden shrink-0 rounded border bg-muted px-1 font-mono text-[10px] sm:inline">
          ⌘K
        </kbd>
      </Button>

      <CommandDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) setQuery('')
        }}
        title={t`Admin search`}
        description={t`Find an order, customer or product, or jump to a page or settings section`}
        // wider than the stock 24rem and hung from the top, so a growing list never shifts the field under the cursor
        className="gap-0 shadow-2xl sm:top-[12vh] sm:max-w-2xl"
      >
        {/* this codebase's CommandDialog doesn't add the <Command> root itself */}
        <Command className="rounded-none! bg-popover p-0">
          <div className="flex items-center gap-3 border-b px-4">
            <SearchIcon className="size-5 shrink-0 text-muted-foreground" />
            <CommandPrimitive.Input
              placeholder={t`Search orders, customers, products and pages…`}
              value={query}
              onValueChange={setQuery}
              className="h-14 min-w-0 flex-1 bg-transparent text-base outline-hidden placeholder:text-muted-foreground"
            />
            {lookingUp ? (
              <span role="status" className="flex shrink-0 items-center">
                <Loader2Icon aria-hidden className="size-4 animate-spin text-muted-foreground" />
                <span className="sr-only">
                  <Trans>Searching</Trans>
                </span>
              </span>
            ) : null}
            <Key className="max-sm:hidden">esc</Key>
          </div>
          <CommandList className="max-h-[min(60vh,28rem)] scroll-py-2 p-2">
            <ResultsAnnouncer />
            {recordError ? (
              <p role="alert" className="flex items-center gap-2 px-3 py-2 text-xs text-destructive">
                <AlertCircleIcon className="size-3.5 shrink-0" />
                <Trans>Couldn't search orders, customers and products - {recordError.message}</Trans>
              </p>
            ) : null}
            <CommandEmpty className="flex flex-col items-center gap-2 py-12 text-muted-foreground">
              {lookingUp ? (
                <span className="text-sm">
                  <Trans>Searching…</Trans>
                </span>
              ) : (
                <>
                  <SearchXIcon className="size-6" />
                  <span className="text-sm">
                    <Trans>Nothing matches “{query}”</Trans>
                  </span>
                </>
              )}
            </CommandEmpty>
            {/* The server already matched these, on fields the row may not show - the typed text as a keyword keeps the
                palette's own filter from dropping them */}
            {records.map((group) => (
              <ResultGroup key={group.heading} heading={group.heading}>
                {group.rows.map((r) => (
                  <Result
                    key={r.key}
                    value={r.key}
                    keywords={[query]}
                    icon={group.icon}
                    title={r.title}
                    context={r.context}
                    onSelect={() => go(r.path)}
                  />
                ))}
              </ResultGroup>
            ))}
            {query === '' && recents.length > 0 && (
              <ResultGroup heading={t`Recent`}>
                {recents.map((p) => (
                  <Result
                    key={`recent-${p.path}`}
                    value={`recent ${p.title}`}
                    icon={p.icon}
                    title={navLabel(p.title)}
                    context={p.context && navLabel(p.context)}
                    onSelect={() => go(p.path)}
                  />
                ))}
              </ResultGroup>
            )}
            <ResultGroup heading={t`Pages`}>
              {pages.map((p) => (
                <Result
                  key={p.path}
                  value={`${p.context ?? ''} ${p.title}`}
                  icon={p.icon}
                  title={navLabel(p.title)}
                  context={p.context && navLabel(p.context)}
                  onSelect={() => go(p.path)}
                />
              ))}
            </ResultGroup>
            <ResultGroup heading={t`Settings`}>
              {sectionsQuery.loading && (
                <CommandItem disabled value="__loading" className="h-11 px-3 text-muted-foreground">
                  <Trans>Loading settings sections…</Trans>
                </CommandItem>
              )}
              {settingsEntries.map((s) => (
                <Result
                  key={s.path}
                  value={`${s.context} ${s.title}`}
                  icon={<SettingsIcon />}
                  title={s.title}
                  context={s.context}
                  onSelect={() => go(s.path)}
                />
              ))}
            </ResultGroup>
          </CommandList>
          {/* What the keys do, for anyone who reached this by clicking */}
          <div className="flex items-center gap-4 border-t px-4 py-2.5 text-xs text-muted-foreground max-sm:hidden">
            <span className="flex items-center gap-1.5">
              <Key>↑</Key>
              <Key>↓</Key>
              <Trans>Move</Trans>
            </span>
            <span className="flex items-center gap-1.5">
              <Key>↵</Key>
              <Trans>Open</Trans>
            </span>
            <span className="flex items-center gap-1.5">
              <Key>esc</Key>
              <Trans>Close</Trans>
            </span>
          </div>
        </Command>
      </CommandDialog>
    </>
  )
}

function ResultGroup({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <CommandGroup
      heading={heading}
      className="p-0 **:[[cmdk-group-heading]]:px-3 **:[[cmdk-group-heading]]:pt-3 **:[[cmdk-group-heading]]:pb-1.5 **:[[cmdk-group-heading]]:text-[11px] **:[[cmdk-group-heading]]:tracking-wider **:[[cmdk-group-heading]]:uppercase"
    >
      {children}
    </CommandGroup>
  )
}

// Says how many rows the list holds after each change, for a screen reader that can't see it update
function ResultsAnnouncer() {
  const count = useCommandState((state) => state.filtered.count)
  return (
    <span aria-live="polite" className="sr-only">
      <Trans>{count} results</Trans>
    </span>
  )
}

// One row: its icon in a tile, its name, where it lives, and the enter key on the row the arrows are on
function Result({
  value,
  keywords,
  icon,
  title,
  context,
  onSelect,
}: {
  value: string
  keywords?: string[]
  icon?: ReactNode
  title: string
  context?: string
  onSelect: () => void
}) {
  return (
    <CommandItem value={value} keywords={keywords} onSelect={onSelect} className="h-11 gap-3 px-3">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/40 text-muted-foreground group-data-selected/command-item:text-foreground [&_svg]:size-4">
        {icon ?? <FileIcon />}
      </span>
      <span className="min-w-0 truncate font-medium">{title}</span>
      {/* As a command-shortcut slot, which is what tells CommandItem to leave off its own check mark */}
      <span data-slot="command-shortcut" className="ml-auto flex shrink-0 items-center gap-3 pl-3 text-xs text-muted-foreground">
        {context ? <span className="truncate">{context}</span> : null}
        <CornerDownLeftIcon className="size-3.5 opacity-0 group-data-selected/command-item:opacity-100" />
      </span>
    </CommandItem>
  )
}

function Key({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        'pointer-events-none inline-flex h-5 min-w-5 items-center justify-center rounded border bg-muted px-1 font-mono text-[10px] text-muted-foreground',
        className,
      )}
    >
      {children}
    </kbd>
  )
}
