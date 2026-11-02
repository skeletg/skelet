import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { LinkablesDocument, type CmsAttributeRowFragment, type CmsPageRowFragment } from '../Cms.gql'
import type { CmsRelationshipTarget } from '@/graphql/schema.types'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import { PAGE_LINKS_LABELS, changedAttributes, pageLinksSchema, pageRelationshipsDiff, toPageLinksValues } from '../schemas/page'
import { type PageTypeAttr } from './shared'

// RelationshipsCard edits entity-reference (relationship) attributes - each attribute gets an ordered
// multi-picker scoped to its target type; edits stage into the page's save registry
export function RelationshipsCard({
  pageId,
  relationshipAttrs,
  relationships,
  canManage,
  onSave,
}: {
  pageId: string
  relationshipAttrs: PageTypeAttr[]
  relationships: CmsPageRowFragment['relationships']
  canManage: boolean
  onSave: (attributeId: string, refs: { targetType: CmsRelationshipTarget; targetId: string }[]) => Promise<unknown>
}) {
  // The parent rebuilds these lists every render - key the baseline on their content
  const key = JSON.stringify([relationshipAttrs.map((a) => a.attribute.id), relationships])
  const baseline = useMemo(
    () => toPageLinksValues(relationshipAttrs, relationships.map((r) => ({ attributeId: r.attributeId, id: r.targetId }))),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [key],
  )
  const { form, draft } = useSectionForm({
    id: 'relationships',
    title: 'Relationships',
    baseline,
    schema: pageLinksSchema,
    diff: pageRelationshipsDiff(relationshipAttrs),
    routeKey: `cms-page-relationships:${pageId}`,
    submit: async (v) => {
      for (const attrId of changedAttributes(baseline, v, true)) {
        const target = relationshipAttrs.find((pta) => pta.attribute.id === attrId)?.attribute.relationshipTarget
        if (!target) continue
        await onSave(attrId, (v.byAttribute[attrId] ?? []).map((id) => ({ targetType: target, targetId: id })))
      }
    },
  })
  const linked = useStore(form.store, (s) => s.values.byAttribute)

  return (
    <div className="grid gap-4">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PAGE_LINKS_LABELS} />}
      {relationshipAttrs.map((pta) => (
        <RelationshipField
          key={pta.attribute.id}
          attribute={pta.attribute}
          value={linked[pta.attribute.id] ?? []}
          disabled={!canManage}
          onChange={(ids) => form.setFieldValue('byAttribute', (prev) => ({ ...prev, [pta.attribute.id]: ids }))}
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
