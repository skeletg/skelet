import { useNavLabel } from '@/nav/nav-i18n'
import { Trans, useLingui } from '@lingui/react/macro'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { ClockIcon, CornerDownLeftIcon, SearchIcon, SettingsIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { useVisiblePages } from '@/nav/use-visible-pages'
import { useConfigScope } from '@/lib/config-scope'
import { useRecentPaths } from '@/lib/recent-pages'
import { useAuth } from '@/shared/auth/auth'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'

// Global admin search (⌘K / Ctrl+K, or the header button): one palette over every sidebar page
// and every settings section/group, so "where does X live" is a keystroke instead of a menu hunt
// Visibility follows the sidebar's own rules - permission-gated and feature-gated entries a user
// can't see in the nav don't surface in search either
export function AdminSearch() {
  const { t } = useLingui()
  const navLabel = useNavLabel()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const navigate = useNavigate()
  const { scopeInput } = useConfigScope()
  const { auth } = useAuth()
  const recentPaths = useRecentPaths(auth?.admin.id ?? '')

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // the settings registry is only worth fetching once the palette opens - it's also
  // scope-shaped, so the results match what the settings page would actually show
  const sectionsQuery = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    skip: !open,
  })

  const { pages } = useVisiblePages()

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

      <CommandDialog open={open} onOpenChange={setOpen} title={t`Admin search`} description={t`Jump to a page or settings section`}>
        {/* this codebase's CommandDialog doesn't add the <Command> root itself */}
        <Command>
        <CommandInput
          placeholder={t`Search pages and settings…`}
          value={query}
          onValueChange={setQuery}
        />
        <CommandList>
          <CommandEmpty><Trans>Nothing found.</Trans></CommandEmpty>
          {query === '' && recents.length > 0 && (
            <CommandGroup heading={t`Recent`}>
              {recents.map((p) => (
                <CommandItem key={`recent-${p.path}`} value={`recent ${p.title}`} onSelect={() => go(p.path)}>
                  <ClockIcon className="size-3.5 text-muted-foreground" />
                  <span>{navLabel(p.title)}</span>
                  {p.context && <span className="ml-auto text-xs text-muted-foreground">{navLabel(p.context)}</span>}
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          <CommandGroup heading={t`Pages`}>
            {pages.map((p) => (
              <CommandItem
                key={p.path}
                value={`${p.context ?? ''} ${p.title}`}
                onSelect={() => go(p.path)}
              >
                <CornerDownLeftIcon className="size-3.5 text-muted-foreground" />
                <span>{navLabel(p.title)}</span>
                {p.context && <span className="ml-auto text-xs text-muted-foreground">{navLabel(p.context)}</span>}
              </CommandItem>
            ))}
          </CommandGroup>
          <CommandGroup heading={t`Settings`}>
            {sectionsQuery.loading && (
              <CommandItem disabled value="__loading">
                <Trans>Loading settings sections…</Trans>
              </CommandItem>
            )}
            {settingsEntries.map((s) => (
              <CommandItem
                key={s.path}
                value={`${s.context} ${s.title}`}
                onSelect={() => go(s.path)}
              >
                <SettingsIcon className="size-3.5 text-muted-foreground" />
                <span>{s.title}</span>
                <span className="ml-auto text-xs text-muted-foreground">{s.context}</span>
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
        </Command>
      </CommandDialog>
    </>
  )
}
