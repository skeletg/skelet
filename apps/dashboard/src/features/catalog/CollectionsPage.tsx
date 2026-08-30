import { useMemo } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { CheckCircle2Icon, CircleOffIcon, PlusIcon, ShapesIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { CollectionDetail } from './CollectionDetail'
import { CollectionCreate } from './CollectionCreate'
import { CollectionsDocument, type CollectionSummary } from './Collections.gql'

function resolveCollectionName(collection: CollectionSummary) {
  const translatedName = collection.content.find((entry) => entry.name.trim())?.name.trim()
  if (translatedName) return translatedName
  return collection.name?.trim() || collection.slug
}

export function CollectionsPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { id: routeId } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const { isGlobal } = useConfigScope()
  const canManage = has('catalog.manage')
  // Collections are global; mutations gated behind GLOBAL scope
  const canEditScope = canManage && isGlobal

  // Locale is attached by Apollo's `localeLink` from the active config
  // scope's `effectiveLocale` - see `lib/apollo.ts` - No per-query var
  const { data, loading, error } = useQuery(CollectionsDocument)
  const collections = data?.collections ?? []

  const columns = useMemo<ColumnDef<CollectionSummary>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (c) => resolveCollectionName(c),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => {
          const c = row.original
          return (
            <div className="flex items-center gap-2.5">
              <div className="grid size-8 shrink-0 place-items-center overflow-hidden rounded-md bg-muted">
                {c.coverImage?.url ? (
                  <img
                    src={c.coverImage.url}
                    alt={c.coverImage.altText}
                    className="size-full object-cover"
                    loading="lazy"
                  />
                ) : (
                  <ShapesIcon className="size-4 text-muted-foreground" />
                )}
              </div>
              <Link to={`/collections/${c.id}`} className="font-medium hover:underline">
                {resolveCollectionName(c)}
              </Link>
            </div>
          )
        },
      },
      {
        accessorKey: 'slug',
        header: 'Slug',
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{row.original.slug}</code>
        ),
      },
      {
        id: 'status',
        header: 'Status',
        size: 120,
        cell: ({ row }) =>
          row.original.isPublished ? (
            <Badge variant="secondary" className="gap-1 text-[10px]">
              <CheckCircle2Icon className="size-3 text-emerald-500" /> Published
            </Badge>
          ) : (
            <Badge variant="outline" className="gap-1 text-[10px]">
              <CircleOffIcon className="size-3 text-muted-foreground/60" /> Draft
            </Badge>
          ),
      },
    ],
    [],
  )

  // Create / detail render full-width (reached from the table) - The list itself
  // is the table
  if (routeId === 'new') return <CollectionCreate />
  if (routeId) return <CollectionDetail key={routeId} />

  return (
    <DataTable
      urlState={urlState}
      columns={columns}
      data={collections}
      loading={loading}
      error={error ? { message: error.message } : null}
      getRowId={(row) => row.id}
      emptyState="No collections yet."
      searchPlaceholder="Search name or slug…"
      toolbarRightSlot={
        canManage && (
          <Button
            size="sm"
            onClick={() => navigate('/collections/new')}
            disabled={!canEditScope}
            title={canEditScope ? undefined : 'Switch the header to GLOBAL to add a collection.'}
          >
            <PlusIcon className="size-4" />
            New collection
          </Button>
        )
      }
    />
  )
}
