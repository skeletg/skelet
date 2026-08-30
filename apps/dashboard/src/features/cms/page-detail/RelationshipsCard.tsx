import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  LinkablesDocument,
  type CmsAttributeRowFragment,
  type CmsPageRowFragment,
} from '../Cms.gql'
import type { CmsRelationshipTarget } from '@/graphql/schema.types'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import { type PageTypeAttr } from './shared'

// RelationshipsCard edits entity-reference (relationship) attributes - Each attribute gets an ordered multi-picker
// scoped to its target type; edits are staged and flushed by the header Save (per the dashboard convention)
export function RelationshipsCard({
  relationshipAttrs,
  relationships,
  canManage,
  onSave,
}: {
  relationshipAttrs: PageTypeAttr[]
  relationships: CmsPageRowFragment['relationships']
  canManage: boolean
  onSave: (
    attributeId: string,
    refs: { targetType: CmsRelationshipTarget; targetId: string }[],
  ) => Promise<unknown>
}) {
  const serverIds = (attrId: string) =>
    relationships.filter((r) => r.attributeId === attrId).map((r) => r.targetId)

  const [draft, setDraft] = useState<Record<string, string[]>>(() => {
    const m: Record<string, string[]> = {}
    for (const pta of relationshipAttrs) m[pta.attribute.id] = serverIds(pta.attribute.id)
    return m
  })
  useEffect(() => {
    const m: Record<string, string[]> = {}
    for (const pta of relationshipAttrs) m[pta.attribute.id] = serverIds(pta.attribute.id)
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(m)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [relationships, relationshipAttrs])

  const changed = useMemo(
    () =>
      relationshipAttrs.filter(
        (pta) => JSON.stringify(serverIds(pta.attribute.id)) !== JSON.stringify(draft[pta.attribute.id] ?? []),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [relationships, relationshipAttrs, draft],
  )

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      changed.map((pta) => ({
        label: pta.attribute.content?.name ?? pta.attribute.slug,
        before: `${serverIds(pta.attribute.id).length} linked`,
        after: `${(draft[pta.attribute.id] ?? []).length} linked`,
      })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [changed, relationships, draft],
  )

  useSaveSection({
    id: 'relationships',
    title: 'Relationships',
    diff,
    save: async () => {
      for (const pta of changed) {
        const target = pta.attribute.relationshipTarget
        if (!target) continue
        const refs = (draft[pta.attribute.id] ?? []).map((id) => ({ targetType: target, targetId: id }))
        await onSave(pta.attribute.id, refs)
      }
    },
  })

  return (
    <div className="grid gap-4">
      {relationshipAttrs.map((pta) => (
        <RelationshipField
          key={pta.attribute.id}
          attribute={pta.attribute}
          value={draft[pta.attribute.id] ?? []}
          disabled={!canManage}
          onChange={(ids) => setDraft((prev) => ({ ...prev, [pta.attribute.id]: ids }))}
        />
      ))}
    </div>
  )
}

// RelationshipField is one attribute's ordered entity picker, backed by the
// linkables search filtered to the attribute's target type
function RelationshipField({
  attribute,
  value,
  disabled,
  onChange,
}: {
  attribute: CmsAttributeRowFragment
  value: string[]
  disabled: boolean
  onChange: (ids: string[]) => void
}) {
  const target = attribute.relationshipTarget
  const [search, setSearch] = useState('')
  const { data, loading } = useQuery(LinkablesDocument, { variables: { search, limit: 20 } })
  const options = useMemo(() => {
    const opts = (data?.linkables ?? [])
      .filter((l) => String(l.entityType) === String(target))
      .map((l) => ({ value: l.entityId, label: l.label || l.path, hint: l.path }))
    // Keep already-selected ids visible even when not in the current search
    for (const id of value) {
      if (!opts.some((o) => o.value === id)) opts.push({ value: id, label: id, hint: 'selected' })
    }
    return opts
  }, [data, target, value])

  return (
    <div className="grid gap-1.5">
      <Label className="flex items-center gap-1">
        {attribute.content?.name ?? attribute.slug}
        <code className="ml-1 text-xs font-normal text-muted-foreground">{attribute.slug}</code>
        {target && (
          <Badge variant="outline" className="text-xs">→ {target}</Badge>
        )}
      </Label>
      <div className={disabled ? 'pointer-events-none opacity-60' : undefined}>
        <AutocompletePicker
          multi
          options={options}
          value={value}
          onChange={(next) => onChange(Array.isArray(next) ? next : [next])}
          onSearch={setSearch}
          loading={loading}
          placeholder={`Link ${String(target ?? 'entity').toLowerCase()}…`}
          emptyMessage="No matches - type to search."
        />
      </div>
    </div>
  )
}
