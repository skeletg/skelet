import { useState } from 'react'
import { CheckIcon, ChevronDownIcon, HouseIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { DEFAULT_LANDING, useLandingPref } from '@/lib/landing'
import { useConfigScope } from '@/lib/config-scope'
import { useScopeDefaultPref } from '@/lib/scope-default'
import { useVisiblePages } from '@/nav/use-visible-pages'

// Landing page: a searchable list of every page this admin can reach, grouped the way the sidebar
// groups them, with the dashboard pinned on top as the default
function LandingPicker() {
  const [open, setOpen] = useState(false)
  const { pages } = useVisiblePages()
  const { landing, title, setLanding } = useLandingPref()
  const groups = new Map<string, typeof pages>()
  for (const page of pages) {
    const key = page.context ?? 'Pages'
    const list = groups.get(key)
    if (list) list.push(page)
    else groups.set(key, [page])
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" role="combobox" aria-expanded={open} className="w-72 justify-between font-normal">
          <span className="truncate">{landing === DEFAULT_LANDING ? 'Dashboard' : title}</span>
          <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        <Command>
          <CommandInput placeholder="Search pages…" className="h-9" />
          <CommandList className="max-h-72">
            <CommandEmpty>No matching page.</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value="Dashboard"
                onSelect={() => {
                  setLanding(DEFAULT_LANDING)
                  setOpen(false)
                }}
              >
                <CheckIcon className={cn('size-4', landing === DEFAULT_LANDING ? 'opacity-100' : 'opacity-0')} />
                <HouseIcon className="size-4 text-muted-foreground" />
                Dashboard
              </CommandItem>
            </CommandGroup>
            {[...groups].map(([group, items]) => (
              <CommandGroup key={group} heading={group}>
                {items.map((page) => (
                  <CommandItem
                    key={page.path}
                    value={`${page.title} ${group}`}
                    onSelect={() => {
                      setLanding(page.path)
                      setOpen(false)
                    }}
                  >
                    <CheckIcon className={cn('size-4', landing === page.path ? 'opacity-100' : 'opacity-0')} />
                    {page.title}
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

// A button rather than a second scope picker: the header already has one, so "remember what I'm
// looking at" is both less UI and unambiguous about what gets stored
function ScopeDefaultControl() {
  const { scope, channels } = useConfigScope()
  const [pref, setPref] = useScopeDefaultPref()
  const channelName = (id: string) => channels.find((c) => c.id === id)?.name ?? 'a channel you no longer have'
  const describe = (s: typeof scope) =>
    s.mode === 'GLOBAL' ? 'Global default' : `${channelName(s.channelId)}${s.locale ? ` · ${s.locale}` : ''}`
  const isCurrent =
    !!pref && pref.mode === scope.mode && pref.channelId === scope.channelId && pref.locale === scope.locale

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="outline" disabled={isCurrent} onClick={() => setPref(scope)}>
        Use my current scope
      </Button>
      {pref && (
        <Button variant="ghost" onClick={() => setPref(undefined)}>
          Clear
        </Button>
      )}
      <span className="text-xs text-muted-foreground">
        {pref ? `New devices start in ${describe(pref)}` : `Currently ${describe(scope)} - nothing saved yet`}
      </span>
    </div>
  )
}

export function StartupSection() {
  return (
    <>
      <DetailPageSection
        title="Landing page"
        description="Where signing in takes you, and what a new workspace tab opens. Opening a link someone shared still goes straight to that page."
      >
        <LandingPicker />
      </DetailPageSection>

      <DetailPageSection
        title="Default scope"
        description="Applied on a device you haven't picked a scope on yet - your other open sessions keep the scope they're in."
      >
        <ScopeDefaultControl />
      </DetailPageSection>
    </>
  )
}
