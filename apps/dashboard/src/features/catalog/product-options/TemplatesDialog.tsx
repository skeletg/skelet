import { useMemo, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { LayersIcon, PencilIcon, PlusIcon, SearchIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { OptionTemplatesDocument } from '../Catalog.gql'
import { NewTemplateForm } from './NewTemplateForm'
import { templatePath, usedByLabel } from './template-path'

// Every configurator template in one place, opened from where they're used - use one here, open one to change it
// everywhere it reaches, or start a new one. A template page opened from here links back
export function TemplatesDialog({
  open,
  onOpenChange,
  usedIds,
  inheritedIds = new Set(),
  onUse,
  canCreate,
  defaultCurrency,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  usedIds: ReadonlySet<string>
  // On a product, the ones its product type hands it
  inheritedIds?: ReadonlySet<string>
  onUse?: (id: string) => Promise<void>
  canCreate: boolean
  defaultCurrency?: string
}) {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { data, loading } = useQuery(OptionTemplatesDocument, { skip: !open })
  const [search, setSearch] = useState('')
  const [creating, setCreating] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const all = useMemo(() => data?.optionTemplates ?? [], [data])
  const shown = useMemo(() => {
    const q = search.trim().toLowerCase()
    return q ? all.filter((t) => `${t.name} ${t.description}`.toLowerCase().includes(q)) : all
  }, [all, search])
  const from = { from: pathname }

  const use = async (id: string) => {
    if (!onUse) return
    setError(null)
    setBusy(id)
    try {
      await onUse(id)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not use the template.')
    } finally {
      setBusy(null)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setCreating(false)
          setError(null)
        }
        onOpenChange(next)
      }}
    >
      <DialogContent className="flex max-h-[min(88vh,48rem)] flex-col gap-4 sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <LayersIcon className="size-5 text-muted-foreground" />
            {creating ? 'New configurator template' : 'Configurator templates'}
          </DialogTitle>
          <DialogDescription>
            {creating
              ? onUse
                ? 'It goes on here straight away - add its options and rules on the next page.'
                : 'Add its options and rules on the next page.'
              : 'Sets of options and rules kept in one place. An edit to a template reaches every product using it.'}
          </DialogDescription>
        </DialogHeader>

        {creating ? (
          <NewTemplateForm
            defaultCurrency={defaultCurrency}
            submitLabel={onUse ? 'Create and use here' : 'Create template'}
            onCancel={() => setCreating(false)}
            onCreated={async (id) => {
              if (onUse) await onUse(id)
              onOpenChange(false)
              setCreating(false)
              navigate(templatePath(id), { state: from })
            }}
          />
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <div className="relative min-w-[12rem] flex-1">
                <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search templates…"
                  aria-label="Search templates"
                  className="h-9 pl-8"
                />
              </div>
              {canCreate ? (
                <Button type="button" size="sm" onClick={() => setCreating(true)}>
                  <PlusIcon className="size-4" />
                  New template
                </Button>
              ) : null}
            </div>
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
            <div className="min-h-0 flex-1 overflow-y-auto">
              {loading && all.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">Loading templates…</p>
              ) : shown.length === 0 ? (
                <p className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  {all.length === 0 ? 'No templates yet. Start one for options many products share - gift wrap, engraving, a care plan.' : 'No template matches.'}
                </p>
              ) : (
                <ul className="divide-y rounded-md border" aria-label="Templates">
                  {shown.map((t) => {
                    const inherited = inheritedIds.has(t.id)
                    const used = inherited || usedIds.has(t.id)
                    const reach = usedByLabel(t)
                    return (
                      <li key={t.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                        <div className="grid min-w-0 flex-1 gap-0.5">
                          <span className="truncate text-sm font-medium">{t.name}</span>
                          {t.description ? <span className="truncate text-xs text-muted-foreground">{t.description}</span> : null}
                          <span className="text-xs text-muted-foreground">
                            {t.optionCount} option{t.optionCount === 1 ? '' : 's'} · {t.ruleCount} rule{t.ruleCount === 1 ? '' : 's'}
                            {reach ? ` · used by ${reach}` : ' · not used yet'}
                          </span>
                        </div>
                        <div className="flex shrink-0 items-center gap-1.5">
                          {inherited ? (
                            <Badge variant="secondary" className="font-normal">From the product type</Badge>
                          ) : used ? (
                            <Badge variant="secondary" className="font-normal">In use here</Badge>
                          ) : onUse ? (
                            <Button type="button" size="sm" variant="outline" disabled={busy !== null} onClick={() => void use(t.id)}>
                              {busy === t.id ? 'Adding…' : 'Use here'}
                            </Button>
                          ) : null}
                          <Button asChild size="sm" variant="ghost">
                            <Link to={templatePath(t.id)} state={from} onClick={() => onOpenChange(false)} aria-label={`Edit ${t.name}`}>
                              <PencilIcon className="size-3.5" />
                              Edit
                            </Link>
                          </Button>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
