import type { ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, CopyPlusIcon, EllipsisIcon, LayersIcon, PlusIcon, SettingsIcon, ShapesIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { OptionTemplatesDocument } from '../Catalog.gql'
import { templatePath } from './template-path'

// One template a product uses: its options read-only under a header that says where they come from, with what can
// be done about it - each action applies right away, like Duplicate, since it restructures rather than edits. One its
// product type hands it is changed on the type, so it offers only the way there
export function TemplateGroup({
  name,
  id,
  count,
  canManage,
  productType,
  note,
  removeBlockedBy = [],
  detachBlockedBy = null,
  onMoveUp = null,
  onMoveDown = null,
  onRemove = () => {},
  onDetach = () => {},
  children,
}: {
  name: string
  id: string
  count: number
  canManage: boolean
  productType?: { id: string; name: string }
  // Shown in place of the options - a pick its product type already hands over shows them in the type's group
  note?: string
  removeBlockedBy?: string[]
  detachBlockedBy?: string | null
  onMoveUp?: (() => void) | null
  onMoveDown?: (() => void) | null
  onRemove?: () => void
  onDetach?: () => void
  children?: ReactNode
}) {
  const { pathname } = useLocation()
  return (
    <section aria-label={productType ? `From the product type ${productType.name}: ${name}` : `From the template ${name}`} className="rounded-md border border-dashed">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-dashed bg-muted/20 px-4 py-2">
        <p className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          {productType ? <ShapesIcon className="size-3.5 shrink-0" /> : <LayersIcon className="size-3.5 shrink-0" />}
          {productType ? (
            <span className="truncate">
              From the product type <span className="font-medium text-foreground">{productType.name}</span> · template{' '}
              <span className="font-medium text-foreground">{name}</span>
            </span>
          ) : (
            <span className="truncate">
              From the template <span className="font-medium text-foreground">{name}</span>
            </span>
          )}
        </p>
        <div className="flex items-center gap-1">
          <Button asChild variant="ghost" size="sm" className="h-7 text-xs">
            <Link to={templatePath(id)} state={{ from: pathname }}>Edit template</Link>
          </Button>
          {productType ? (
            <Button asChild variant="ghost" size="sm" className="h-7 text-xs">
              <Link to={`/catalog/product-types/${productType.id}`}>Change on the type</Link>
            </Button>
          ) : canManage ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`Actions for the template ${name}`}>
                  <EllipsisIcon className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-72">
                <DropdownMenuItem disabled={!onMoveUp} onSelect={() => onMoveUp?.()}>
                  <ArrowUpIcon className="size-4" />
                  Show earlier
                </DropdownMenuItem>
                <DropdownMenuItem disabled={!onMoveDown} onSelect={() => onMoveDown?.()}>
                  <ArrowDownIcon className="size-4" />
                  Show later
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem disabled={!!detachBlockedBy} onSelect={onDetach}>
                  <CopyPlusIcon className="size-4" />
                  <span className="grid">
                    Detach as own options
                    <span className="text-xs text-muted-foreground">{detachBlockedBy ?? 'Copies them onto this product to change here'}</span>
                  </span>
                </DropdownMenuItem>
                <DropdownMenuItem variant="destructive" disabled={removeBlockedBy.length > 0} onSelect={onRemove}>
                  <Trash2Icon className="size-4" />
                  <span className="grid">
                    Remove from this product
                    {removeBlockedBy.length > 0 ? (
                      <span className="text-xs text-muted-foreground">Used by {removeBlockedBy.join(', ')}</span>
                    ) : null}
                  </span>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : null}
        </div>
      </div>
      {note ? (
        <p className="px-4 py-3 text-xs text-muted-foreground">{note}</p>
      ) : count === 0 ? (
        <p className="px-4 py-3 text-xs text-muted-foreground">This template has no options yet.</p>
      ) : (
        <ul className="divide-y">{children}</ul>
      )}
    </section>
  )
}

export function AddTemplateMenu({
  usedIds,
  onAdd,
  onManage,
}: {
  usedIds: ReadonlySet<string>
  onAdd: (id: string) => void
  // Opens every template in one place - to create one, or to edit one
  onManage?: () => void
}) {
  const { data, loading } = useQuery(OptionTemplatesDocument)
  const free = (data?.optionTemplates ?? []).filter((t) => !usedIds.has(t.id))
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" size="sm" variant="outline">
          <PlusIcon className="size-4" />
          Use a template
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
          A template's options show first, and stay in step with it
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {loading ? <DropdownMenuItem disabled>Loading…</DropdownMenuItem> : null}
        {free.map((t) => (
          <DropdownMenuItem key={t.id} onSelect={() => onAdd(t.id)}>
            <LayersIcon className="size-4" />
            <span className="grid min-w-0">
              <span className="truncate">{t.name}</span>
              <span className="text-xs text-muted-foreground">
                {t.optionCount} option{t.optionCount === 1 ? '' : 's'} · {t.ruleCount} rule{t.ruleCount === 1 ? '' : 's'}
              </span>
            </span>
          </DropdownMenuItem>
        ))}
        {!loading && free.length === 0 ? (
          <DropdownMenuItem disabled={!onManage} onSelect={() => onManage?.()}>
            {data?.optionTemplates.length ? 'Every template is in use here' : 'No templates yet - create one'}
          </DropdownMenuItem>
        ) : null}
        {onManage ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onManage}>
              <SettingsIcon className="size-4" />
              Manage templates…
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
