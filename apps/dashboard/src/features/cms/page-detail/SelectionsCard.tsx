import { useMemo } from 'react'
import { useStore } from '@tanstack/react-form'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { type CmsAttributeRowFragment } from '../Cms.gql'
import { PAGE_LINKS_LABELS, changedAttributes, pageLinksSchema, pageSelectionsDiff, toPageLinksValues } from '../schemas/page'
import { type PageTypeAttr } from './shared'

// ---------- Selections card ----------

// Options for the page's select / multiselect attributes - staged into the page's save registry,
// one write per changed attribute when the header Save runs
export function SelectionsCard({
  pageId,
  selectAttrs,
  selections,
  canManage,
  onSave,
}: {
  pageId: string
  selectAttrs: PageTypeAttr[]
  selections: { attributeId: string; valueId: string }[]
  canManage: boolean
  onSave: (attributeId: string, valueIds: string[]) => Promise<unknown>
}) {
  // The parent rebuilds these lists every render - key the baseline on their content
  const key = JSON.stringify([selectAttrs.map((a) => a.attribute.id), selections])
  const baseline = useMemo(
    () => toPageLinksValues(selectAttrs, selections.map((s) => ({ attributeId: s.attributeId, id: s.valueId }))),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [key],
  )
  const { form, draft } = useSectionForm({
    id: 'selections',
    title: 'Selections',
    baseline,
    schema: pageLinksSchema,
    diff: pageSelectionsDiff(selectAttrs),
    routeKey: `cms-page-selections:${pageId}`,
    submit: async (v) => {
      // One mutation per changed attribute, in turn, so a failure can't interleave writes
      for (const attrId of changedAttributes(baseline, v, false)) await onSave(attrId, v.byAttribute[attrId] ?? [])
    },
  })
  const chosen = useStore(form.store, (s) => s.values.byAttribute)

  function toggle(attribute: CmsAttributeRowFragment, valueId: string) {
    form.setFieldValue('byAttribute', (prev) => {
      const current = prev[attribute.id] ?? []
      const has = current.includes(valueId)
      // A single select swaps its one value; a multiselect adds or drops it
      const next = attribute.inputType === 'SELECT' ? (has ? [] : [valueId]) : has ? current.filter((id) => id !== valueId) : [...current, valueId]
      return { ...prev, [attribute.id]: next }
    })
  }

  return (
    <fieldset disabled={!canManage} className="grid gap-4">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PAGE_LINKS_LABELS} />}
      {selectAttrs.map((pta) => {
        const a = pta.attribute
        const current = new Set(chosen[a.id] ?? [])
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
                    <Checkbox id={`v-${v.id}`} checked={current.has(v.id)} onCheckedChange={() => toggle(a, v.id)} />
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
    </fieldset>
  )
}
