import { useMemo } from 'react'
import { useStore } from '@tanstack/react-form'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { type CmsPageRowFragment } from '../Cms.gql'
import { DynamicField, type FieldValue } from '../DynamicFieldRenderer'
import { PAGE_BASICS_LABELS, pageBasicsDiff, pageBasicsSchema, toPageBasicsInput, toPageBasicsValues } from '../schemas/page'
import { type PageTypeAttr } from './shared'

// ---------- Basics card ----------

// The slug and the non-translated attribute values - staged into the page's save registry,
// with its own stored draft
export function BasicsCard({
  page,
  scalarAttrs,
  canManage,
  onSave,
}: {
  page: CmsPageRowFragment
  scalarAttrs: PageTypeAttr[]
  canManage: boolean
  onSave: (input: ReturnType<typeof toPageBasicsInput>) => Promise<unknown>
}) {
  const baseline = useMemo(() => toPageBasicsValues(page), [page])
  const { form, draft } = useSectionForm({
    id: 'basics',
    title: 'Basics & non-translated fields',
    baseline,
    schema: pageBasicsSchema,
    diff: pageBasicsDiff(scalarAttrs),
    routeKey: `cms-page-basics:${page.id}`,
    submit: async (v) => {
      await onSave(toPageBasicsInput(baseline, v, page.id))
    },
  })
  const data = useStore(form.store, (s) => s.values.data) as Record<string, FieldValue>

  return (
    <fieldset disabled={!canManage} className="grid gap-4">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PAGE_BASICS_LABELS} />}
      <form.AppField name="slug">
        {(f) => <f.TextField id="slug" label="Slug" hint="Changing it leaves a redirect from the old URL." />}
      </form.AppField>
      {scalarAttrs.map((pta) => (
        <DynamicField
          key={pta.attribute.id}
          attribute={pta.attribute}
          isRequired={pta.isRequired || pta.attribute.isRequiredDefault}
          disabled={!canManage}
          value={data[pta.attribute.slug]}
          onChange={(v) => form.setFieldValue('data', (prev) => ({ ...prev, [pta.attribute.slug]: v }))}
        />
      ))}
    </fieldset>
  )
}
