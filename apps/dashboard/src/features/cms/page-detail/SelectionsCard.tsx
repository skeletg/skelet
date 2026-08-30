import { useEffect, useMemo, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { type CmsAttributeRowFragment } from '../Cms.gql'
import { type PageTypeAttr } from './shared'

// ---------- Selections card ----------

function groupSelections(selections: { attributeId: string; valueId: string }[]): Record<string, string[]> {
  const out: Record<string, string[]> = {}
  for (const s of selections) (out[s.attributeId] ??= []).push(s.valueId)
  return out
}

export function SelectionsCard({
  selectAttrs,
  selections,
  canManage,
  onSave,
}: {
  selectAttrs: PageTypeAttr[]
  selections: { attributeId: string; valueId: string }[]
  canManage: boolean
  onSave: (attributeId: string, valueIds: string[]) => Promise<unknown>
}) {
  // Staged per-attribute value-id sets; toggles only touch this draft and
  // the header Save flushes the changed attributes
  const [draft, setDraft] = useState<Record<string, string[]>>(() => groupSelections(selections))

  useEffect(() => {
    // Re-seed the per-attribute draft from the server selections
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(groupSelections(selections))
  }, [selections])

  function toggle(attribute: CmsAttributeRowFragment, valueId: string) {
    setDraft((prev) => {
      const current = prev[attribute.id] ?? []
      const has = current.includes(valueId)
      const next =
        attribute.inputType === 'SELECT'
          ? has
            ? []
            : [valueId]
          : has
          ? current.filter((id) => id !== valueId)
          : [...current, valueId]
      return { ...prev, [attribute.id]: next }
    })
  }

  const changedAttributeIds = useMemo(() => {
    const server = groupSelections(selections)
    return selectAttrs
      .map((pta) => pta.attribute.id)
      .filter((attrId) => {
        const before = [...(server[attrId] ?? [])].sort()
        const after = [...(draft[attrId] ?? [])].sort()
        return JSON.stringify(before) !== JSON.stringify(after)
      })
  }, [selections, selectAttrs, draft])

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const server = groupSelections(selections)
    return changedAttributeIds.map((attrId) => {
      const a = selectAttrs.find((pta) => pta.attribute.id === attrId)!.attribute
      const names = (ids: string[]) =>
        ids.map((id) => a.values.find((v) => v.id === id)?.slug ?? id).join(', ')
      return {
        label: a.content?.name ?? a.slug,
        before: names(server[attrId] ?? []),
        after: names(draft[attrId] ?? []),
      }
    })
  }, [changedAttributeIds, selections, selectAttrs, draft])

  useSaveSection({
    id: 'selections',
    title: 'Selections',
    diff,
    save: async () => {
      // One mutation per changed attribute, sequential so a failure
      // doesn't leave silently-interleaved writes
      for (const attrId of changedAttributeIds) {
        await onSave(attrId, draft[attrId] ?? [])
      }
    },
  })

  return (
    <div className="grid gap-4">
      {selectAttrs.map((pta) => {
        const a = pta.attribute
        const current = new Set(draft[a.id] ?? [])
        return (
          <div key={a.id} className="grid gap-2">
            <Label className="flex items-center gap-1">
              {a.content?.name ?? a.slug}
              {(pta.isRequired || a.isRequiredDefault) && <span className="text-destructive">*</span>}
              <Badge variant="outline" className="ml-1 text-xs">
                {a.inputType.toLowerCase()}
              </Badge>
            </Label>
            {a.values.length === 0 ? (
              <p className="text-xs text-muted-foreground">No options defined for this attribute.</p>
            ) : (
              <ul className="grid gap-1 md:grid-cols-2">
                {a.values.map((v) => (
                  <li key={v.id} className="flex items-center gap-2 rounded-md border px-2 py-1.5">
                    <Checkbox
                      id={`v-${v.id}`}
                      checked={current.has(v.id)}
                      onCheckedChange={() => toggle(a, v.id)}
                      disabled={!canManage}
                    />
                    <Label htmlFor={`v-${v.id}`} className="flex-1 cursor-pointer text-sm font-normal">
                      {v.slug}
                    </Label>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      })}
    </div>
  )
}
