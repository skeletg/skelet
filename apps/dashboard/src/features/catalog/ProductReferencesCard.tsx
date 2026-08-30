import { useEffect, useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, ExternalLinkIcon, XIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import type { AttributeReferenceTarget } from '@/graphql/schema.types'
import {
  ProductDocument,
  ReferenceCandidatesDocument,
  SetProductAttributeReferencesDocument,
} from './Catalog.gql'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from './productSaveRegistry'

// One reference-type attribute value on a product (from the Product query)
export type ProductReferenceRow = {
  attribute: {
    id: string
    slug: string
    name: string
    relationshipTarget?: AttributeReferenceTarget | null
  }
  references: { targetType: string; targetId: string; label: string; url: string }[]
}

// Admin detail route per target type — the linked item links into admin, not
// the storefront (no storefront base URL needed)
const ADMIN_PATH: Record<AttributeReferenceTarget, string> = {
  PRODUCT: '/products/',
  CATEGORY: '/categories/',
  COLLECTION: '/collections/',
}

// ProductReferencesCard edits a product's reference-attribute values - Each
// attribute shows an ordered list of linked entities (reorder / remove / open)
// plus a type-scoped search to add more - Edits stage into the product Save
// flow and flush through setProductAttributeReferences (one call per changed
// attribute) - Mirrors the CMS RelationshipsCard, hardened for the product side
export function ProductReferencesCard({
  productId,
  referenceAttrs,
  canManage,
}: {
  productId: string
  referenceAttrs: ProductReferenceRow[]
  canManage: boolean
}) {
  const [setRefs] = useMutation(SetProductAttributeReferencesDocument, {
    refetchQueries: [{ query: ProductDocument, variables: { id: productId } }],
    awaitRefetchQueries: true,
  })

  const serverIds = (attrId: string) =>
    referenceAttrs.find((r) => r.attribute.id === attrId)?.references.map((x) => x.targetId) ?? []

  const [draft, setDraft] = useState<Record<string, string[]>>(() => {
    const m: Record<string, string[]> = {}
    for (const row of referenceAttrs) m[row.attribute.id] = serverIds(row.attribute.id)
    return m
  })
  useEffect(() => {
    const m: Record<string, string[]> = {}
    for (const row of referenceAttrs) m[row.attribute.id] = serverIds(row.attribute.id)
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(m)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [referenceAttrs])

  const changed = useMemo(
    () =>
      referenceAttrs.filter(
        (row) =>
          JSON.stringify(serverIds(row.attribute.id)) !==
          JSON.stringify(draft[row.attribute.id] ?? []),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [referenceAttrs, draft],
  )

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      changed.map((row) => ({
        label: row.attribute.name,
        before: `${serverIds(row.attribute.id).length} linked`,
        after: `${(draft[row.attribute.id] ?? []).length} linked`,
      })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [changed, draft],
  )

  useProductSaveSection({
    id: 'references',
    title: 'References',
    diff,
    save: async () => {
      for (const row of changed) {
        const target = row.attribute.relationshipTarget
        if (!target) continue
        const refs = (draft[row.attribute.id] ?? []).map((id) => ({ targetType: target, targetId: id }))
        await setRefs({ variables: { productId, attributeId: row.attribute.id, refs } })
      }
    },
  })

  if (referenceAttrs.length === 0) return null

  return (
    <div className="grid gap-6">
      {referenceAttrs.map((row) => (
        <ReferenceField
          key={row.attribute.id}
          productId={productId}
          row={row}
          value={draft[row.attribute.id] ?? []}
          disabled={!canManage}
          onChange={(ids) => setDraft((prev) => ({ ...prev, [row.attribute.id]: ids }))}
        />
      ))}
    </div>
  )
}

type EntityInfo = { label: string; path: string }

function ReferenceField({
  productId,
  row,
  value,
  disabled,
  onChange,
}: {
  productId: string
  row: ProductReferenceRow
  value: string[]
  disabled: boolean
  onChange: (ids: string[]) => void
}) {
  const target = row.attribute.relationshipTarget
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounced(search, 250)

  const { data, loading } = useQuery(ReferenceCandidatesDocument, {
    variables: { target: target ?? 'PRODUCT', search: debouncedSearch, limit: 20 },
    skip: !target,
  })
  const candidates = useMemo(
    () => (data as { referenceCandidates?: { entityId: string; label: string; path: string }[] } | undefined)?.referenceCandidates ?? [],
    [data],
  )

  // id → {label, path} - Seeded from the server-resolved references, extended as candidates stream in — so a linked
  // item always renders a real label, and a just-added one keeps its label before the save/refetch lands
  const [info, setInfo] = useState<Record<string, EntityInfo>>(() => {
    const m: Record<string, EntityInfo> = {}
    for (const r of row.references) m[r.targetId] = { label: r.label, path: r.url }
    return m
  })
  useEffect(() => {
    if (candidates.length === 0) return
    // Accumulate resolved labels/paths across searches so a linked item keeps
    // its label even after the search term that surfaced it changes
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setInfo((prev) => {
      const next = { ...prev }
      for (const c of candidates) next[c.entityId] = { label: c.label, path: c.path }
      return next
    })
  }, [candidates])

  // Exclude self and already-linked from the add list
  const options = candidates
    .filter((c) => c.entityId !== productId && !value.includes(c.entityId))
    .map((c) => ({ value: c.entityId, label: c.label, hint: c.path }))

  const add = (id: string) => {
    if (!id || id === productId || value.includes(id)) return
    onChange([...value, id])
  }
  const remove = (id: string) => onChange(value.filter((v) => v !== id))
  const move = (index: number, dir: -1 | 1) => {
    const to = index + dir
    if (to < 0 || to >= value.length) return
    const next = [...value]
    ;[next[index], next[to]] = [next[to], next[index]]
    onChange(next)
  }

  const adminBase = target ? ADMIN_PATH[target] : undefined

  return (
    <div className="grid gap-2">
      <Label className="flex items-center gap-1.5">
        {row.attribute.name}
        <code className="text-xs font-normal text-muted-foreground">{row.attribute.slug}</code>
        {target && (
          <Badge variant="outline" className="text-xs">
            → {target.toLowerCase()}
          </Badge>
        )}
      </Label>

      {value.length === 0 ? (
        <p className="rounded-md border border-dashed px-3 py-2 text-sm text-muted-foreground">
          No links yet.
        </p>
      ) : (
        <ul className="grid gap-1.5">
          {value.map((id, i) => {
            const meta = info[id]
            return (
              <li
                key={id}
                className="flex items-center gap-2 rounded-md border bg-card/50 px-3 py-1.5 text-sm"
              >
                <span className="min-w-0 flex-1 truncate">
                  {adminBase ? (
                    <Link to={`${adminBase}${id}`} className="font-medium hover:underline">
                      {meta?.label ?? id}
                    </Link>
                  ) : (
                    <span className="font-medium">{meta?.label ?? id}</span>
                  )}
                  {meta?.path ? (
                    <code className="ml-2 text-xs text-muted-foreground">/{meta.path}</code>
                  ) : null}
                </span>
                {meta?.path ? (
                  <a
                    href={`/${meta.path}`}
                    target="_blank"
                    rel="noreferrer"
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    aria-label="Open on storefront"
                    title="Open on storefront"
                  >
                    <ExternalLinkIcon className="size-3.5" />
                  </a>
                ) : null}
                {!disabled && (
                  <div className="flex shrink-0 items-center">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      disabled={i === 0}
                      onClick={() => move(i, -1)}
                      aria-label="Move up"
                    >
                      <ArrowUpIcon className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      disabled={i === value.length - 1}
                      onClick={() => move(i, 1)}
                      aria-label="Move down"
                    >
                      <ArrowDownIcon className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7 text-muted-foreground hover:text-destructive"
                      onClick={() => remove(id)}
                      aria-label={`Remove ${meta?.label ?? id}`}
                    >
                      <XIcon className="size-3.5" />
                    </Button>
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}

      {!disabled && (
        <AutocompletePicker
          value={null}
          options={options}
          onChange={(next) => add(Array.isArray(next) ? next[0] : next)}
          onSearch={setSearch}
          loading={loading}
          placeholder={`Add ${String(target ?? 'entity').toLowerCase()}…`}
          emptyMessage={loading ? 'Searching…' : 'No matches — type to search.'}
        />
      )}
    </div>
  )
}
