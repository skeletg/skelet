import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { BookmarkIcon, CheckIcon, PencilIcon, StarIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { usePreference, usePrefsLoaded } from '@/lib/admin-prefs'
import { randomId } from '@/lib/random-id'

// Named list views: a view snapshots the URL-held list state (search, filters, sort, page size)
// under the page's own encoding, so applying one is just writing those params back - the starred
// view auto-applies when the page opens with a CLEAN url, so deep links and hand-tuned filters
// always win over the default

export type SavedView = {
  id: string
  name: string
  q?: string
  f?: string
  s?: string
  size?: string
}

export type ViewsPref = { views: SavedView[]; defaultId?: string }

const EMPTY: ViewsPref = { views: [] }
const PARAMS = ['q', 'f', 's', 'size'] as const
const MAX_VIEWS = 20

export function SavedViews({ viewsKey, paramKey = '' }: { viewsKey: string; paramKey?: string }) {
  const [params, setParams] = useSearchParams()
  const [store, setStore] = usePreference<ViewsPref>(`views:${viewsKey}`, EMPTY)
  const loaded = usePrefsLoaded()
  const [open, setOpen] = useState(false)
  const [draftName, setDraftName] = useState('')
  const [renamingId, setRenamingId] = useState<string | null>(null)

  const p = (n: string) => (paramKey ? `${paramKey}_${n}` : n)

  const apply = (view: SavedView) => {
    const next = new URLSearchParams(params)
    for (const n of PARAMS) {
      const v = view[n]
      if (v) next.set(p(n), v)
      else next.delete(p(n))
    }
    next.delete(p('page')) // a different filter set starts back on page one
    setParams(next, { replace: true })
  }

  // the starred view fills in ONLY when the url arrived clean - any present list param means a
  // deep link or an interaction that already happened, and stomping it would lose the user's intent
  const autoApplied = useRef(false)
  useEffect(() => {
    if (!loaded || autoApplied.current) return
    autoApplied.current = true
    const clean = [...PARAMS, 'page'].every((n) => !params.get(p(n)))
    const def = store.views.find((v) => v.id === store.defaultId)
    if (clean && def) apply(def)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- one-shot by design, the ref gates re-runs
  }, [loaded])

  const saveCurrent = () => {
    const name = draftName.trim()
    if (!name) return
    const view: SavedView = { id: randomId(), name }
    for (const n of PARAMS) {
      const v = params.get(p(n))
      if (v) view[n] = v
    }
    setStore((prev) => ({ ...prev, views: [...prev.views, view].slice(0, MAX_VIEWS) }))
    setDraftName('')
  }

  const rename = (id: string) => {
    const name = draftName.trim()
    if (!name) return
    setStore((prev) => ({
      ...prev,
      views: prev.views.map((v) => (v.id === id ? { ...v, name } : v)),
    }))
    setRenamingId(null)
    setDraftName('')
  }

  const remove = (id: string) =>
    setStore((prev) => ({
      views: prev.views.filter((v) => v.id !== id),
      defaultId: prev.defaultId === id ? undefined : prev.defaultId,
    }))

  const toggleDefault = (id: string) =>
    setStore((prev) => ({ ...prev, defaultId: prev.defaultId === id ? undefined : id }))

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8 gap-1.5">
          <BookmarkIcon className="size-3.5" />
          Views
          {store.views.length > 0 && (
            <span className="text-xs text-muted-foreground">{store.views.length}</span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-2">
        {store.views.length === 0 ? (
          <p className="px-2 py-1.5 text-xs text-muted-foreground">
            No saved views yet - set up filters and save them below.
          </p>
        ) : (
          <ul className="grid gap-0.5">
            {store.views.map((v) => (
              <li key={v.id} className="group/view flex items-center gap-1">
                {renamingId === v.id ? (
                  <form
                    className="flex flex-1 items-center gap-1"
                    onSubmit={(e) => {
                      e.preventDefault()
                      rename(v.id)
                    }}
                  >
                    <Input
                      autoFocus
                      value={draftName}
                      onChange={(e) => setDraftName(e.target.value)}
                      className="h-7 text-xs"
                    />
                    <Button type="submit" variant="ghost" size="icon" className="size-7">
                      <CheckIcon className="size-3.5" />
                    </Button>
                  </form>
                ) : (
                  <>
                    <button
                      type="button"
                      onClick={() => {
                        apply(v)
                        setOpen(false)
                      }}
                      className="min-w-0 flex-1 truncate rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
                    >
                      {v.name}
                    </button>
                    <button
                      type="button"
                      title={store.defaultId === v.id ? 'Default view - click to unset' : 'Apply automatically when the page opens'}
                      onClick={() => toggleDefault(v.id)}
                      className="rounded-md p-1 hover:bg-muted"
                    >
                      <StarIcon
                        className={cn(
                          'size-3.5',
                          store.defaultId === v.id
                            ? 'fill-primary text-primary'
                            : 'text-muted-foreground opacity-0 group-hover/view:opacity-100',
                        )}
                      />
                    </button>
                    <button
                      type="button"
                      title="Rename"
                      onClick={() => {
                        setRenamingId(v.id)
                        setDraftName(v.name)
                      }}
                      className="rounded-md p-1 text-muted-foreground opacity-0 group-hover/view:opacity-100 hover:bg-muted"
                    >
                      <PencilIcon className="size-3.5" />
                    </button>
                    <button
                      type="button"
                      title="Delete view"
                      onClick={() => remove(v.id)}
                      className="rounded-md p-1 text-muted-foreground opacity-0 group-hover/view:opacity-100 hover:bg-muted"
                    >
                      <Trash2Icon className="size-3.5" />
                    </button>
                  </>
                )}
              </li>
            ))}
          </ul>
        )}
        {renamingId === null && (
          <form
            className="mt-2 flex items-center gap-1.5 border-t pt-2"
            onSubmit={(e) => {
              e.preventDefault()
              saveCurrent()
            }}
          >
            <Input
              value={draftName}
              onChange={(e) => setDraftName(e.target.value)}
              placeholder="Save current view as…"
              className="h-7 text-xs"
            />
            <Button type="submit" size="sm" className="h-7" disabled={!draftName.trim() || store.views.length >= MAX_VIEWS}>
              Save
            </Button>
          </form>
        )}
      </PopoverContent>
    </Popover>
  )
}
