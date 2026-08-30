import { useEffect, useMemo, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { type CmsPageRowFragment } from '../Cms.gql'
import { DynamicField, type FieldValue } from '../DynamicFieldRenderer'
import { dynamicFieldDiff, type PageTypeAttr } from './shared'

// ---------- Basics card ----------

export function BasicsCard({
  page,
  scalarAttrs,
  canManage,
  onSave,
}: {
  page: CmsPageRowFragment
  scalarAttrs: PageTypeAttr[]
  canManage: boolean
  onSave: (input: {
    id: string
    slug: string | null
    seoNoindex: boolean
    data: Record<string, FieldValue>
  }) => Promise<unknown>
}) {
  const [slug, setSlug] = useState(page.slug)
  const [seoNoindex, setSeoNoindex] = useState(page.seoNoindex)
  const [scalarData, setScalarData] = useState<Record<string, FieldValue>>(
    (page.data as Record<string, FieldValue>) ?? {},
  )

  useEffect(() => {
    // The intended "sync from external source" use of an effect
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSlug(page.slug)
    setSeoNoindex(page.seoNoindex)
    setScalarData((page.data as Record<string, FieldValue>) ?? {})
    // Keyed on page IDENTITY, not the object - every refetch produces a new object and would
    // wipe staged drafts mid-edit - Post-save needs no reset: the refetched baseline equals the
    // draft, so the diff empties on its own
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page.id])

  // Staged save: field edits register with the page save registry; the
  // header Save + confirm modal applies them
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    if (slug !== page.slug) out.push({ label: 'slug', before: page.slug, after: slug })
    if (seoNoindex !== page.seoNoindex) {
      out.push({ label: 'noindex', before: String(page.seoNoindex), after: String(seoNoindex) })
    }
    out.push(...dynamicFieldDiff(scalarAttrs, (page.data as Record<string, FieldValue>) ?? {}, scalarData))
    return out
  }, [page, scalarAttrs, slug, seoNoindex, scalarData])

  useSaveSection({
    id: 'basics',
    title: 'Basics & non-translated fields',
    diff,
    save: async () => {
      await onSave({
        id: page.id,
        slug: slug !== page.slug ? slug : null,
        seoNoindex,
        data: scalarData,
      })
    },
  })

  return (
    <div className="grid gap-4">
      <div className="grid gap-1.5">
        <Label htmlFor="slug">Slug</Label>
        <Input id="slug" value={slug} onChange={(e) => setSlug(e.target.value)} disabled={!canManage} />
      </div>
      <div className="flex items-center gap-2">
        <Switch id="seo-noindex" checked={seoNoindex} onCheckedChange={setSeoNoindex} disabled={!canManage} />
        <Label htmlFor="seo-noindex">Hide from search engines (noindex)</Label>
      </div>
      {scalarAttrs.map((pta) => (
        <DynamicField
          key={pta.attribute.id}
          attribute={pta.attribute}
          isRequired={pta.isRequired || pta.attribute.isRequiredDefault}
          disabled={!canManage}
          value={scalarData[pta.attribute.slug]}
          onChange={(v) => setScalarData((prev) => ({ ...prev, [pta.attribute.slug]: v }))}
        />
      ))}
    </div>
  )
}
