import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { TagIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { CmsPageDocument, CmsTagsDocument, CreateCmsTagDocument, UpdateCmsPageDocument } from '../Cms.gql'
import { PAGE_TAGS_LABELS, pageTagsDiff, pageTagsSchema } from '../schemas/page'

// Tag picker - assigns tags from the managed library (CMS -> Tags) by toggling chips; a new tag can be
// created inline (added to the library at once, and selected). The selection stages into the page Save
export function CmsPageTagsSection({
  pageId,
  tags,
  canManage,
  refetchVariables,
}: {
  pageId: string
  tags: string[]
  canManage: boolean
  refetchVariables: { id: string }
}) {
  const [update] = useMutation(UpdateCmsPageDocument, { refetchQueries: [{ query: CmsPageDocument, variables: refetchVariables }] })
  const { data: tagData } = useQuery(CmsTagsDocument)
  const [createTag] = useMutation(CreateCmsTagDocument, { refetchQueries: [{ query: CmsTagsDocument }] })
  const library = useMemo(() => tagData?.cmsTags ?? [], [tagData])
  const nameBySlug = useMemo(() => Object.fromEntries(library.map((t) => [t.slug, t.name])) as Record<string, string>, [library])
  const [newName, setNewName] = useState('')

  const baseline = useMemo(() => ({ tags: [...tags] }), [tags])
  const { form, draft } = useSectionForm({
    id: 'tags',
    title: 'Tags',
    baseline,
    schema: pageTagsSchema,
    diff: pageTagsDiff((slug) => nameBySlug[slug] ?? slug),
    routeKey: `cms-page-tags:${pageId}`,
    submit: async (v) => {
      await update({ variables: { input: { id: pageId, tags: v.tags } } })
    },
  })
  const chosen = useStore(form.store, (s) => s.values.tags)

  const toggle = (slug: string) => form.setFieldValue('tags', (prev) => (prev.includes(slug) ? prev.filter((x) => x !== slug) : [...prev, slug]))

  async function onCreate() {
    const name = newName.trim()
    if (!name) return
    const res = await createTag({ variables: { name } })
    const slug = res.data?.createCMSTag.slug
    if (slug) form.setFieldValue('tags', (prev) => (prev.includes(slug) ? prev : [...prev, slug]))
    setNewName('')
  }

  // Selected slugs the library doesn't know still render, so they can be removed
  const orphans = chosen.filter((s) => !(s in nameBySlug))
  const selected = new Set(chosen)
  const chip = (on: boolean) =>
    cn(
      'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs transition-colors',
      on ? 'border-foreground/40 bg-muted text-foreground' : 'text-muted-foreground hover:bg-muted/50',
    )

  return (
    <div className="grid gap-3">
      {draft.restoredAt !== null && <RestoredDraftBanner draft={draft} labels={PAGE_TAGS_LABELS} />}
      {library.length === 0 && orphans.length === 0 ? (
        <p className="text-sm text-muted-foreground">No tags in the library yet - create one below, or in CMS → Tags.</p>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {library.map((t) => (
            <button key={t.slug} type="button" disabled={!canManage} aria-pressed={selected.has(t.slug)} onClick={() => toggle(t.slug)} className={chip(selected.has(t.slug))}>
              <TagIcon className="size-3" />
              {t.name}
            </button>
          ))}
          {orphans.map((s) => (
            <button key={s} type="button" disabled={!canManage} aria-pressed onClick={() => toggle(s)} title="Not in the tag library" className={chip(true)}>
              <TagIcon className="size-3" />
              {s}
            </button>
          ))}
        </div>
      )}
      {canManage && (
        <div className="flex items-center gap-2">
          <Input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                void onCreate()
              }
            }}
            placeholder="Create & assign a new tag…"
            className="h-8 max-w-xs"
          />
          <Button className="h-10" type="button" variant="outline" size="sm" disabled={!newName.trim()} onClick={() => void onCreate()}>
            Create
          </Button>
        </div>
      )}
    </div>
  )
}
