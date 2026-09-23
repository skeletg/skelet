import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { TagIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  CmsPageDocument,
  CmsTagsDocument,
  CreateCmsTagDocument,
  UpdateCmsPageDocument,
} from '../Cms.gql'

// Tag picker - Assigns tags from the managed library (CMS -> Tags) by toggling chips; a new tag can be created inline
// (added to the library + selected) The selection (a set of tag slugs) stages into the global Save
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
  const [update] = useMutation(UpdateCmsPageDocument, {
    refetchQueries: [{ query: CmsPageDocument, variables: refetchVariables }],
  })
  const { data: tagData } = useQuery(CmsTagsDocument)
  const [createTag] = useMutation(CreateCmsTagDocument, { refetchQueries: [{ query: CmsTagsDocument }] })
  const library = useMemo(() => tagData?.cmsTags ?? [], [tagData])
  const nameBySlug = useMemo(() => {
    const m: Record<string, string> = {}
    for (const t of library) m[t.slug] = t.name
    return m
  }, [library])

  const server = useMemo(() => [...tags].sort(), [tags])
  const [draft, setDraft] = useState<string[]>(() => [...tags])
  const [newName, setNewName] = useState('')
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft([...tags])
  }, [tags])

  const toggle = (slug: string) =>
    setDraft((prev) => (prev.includes(slug) ? prev.filter((x) => x !== slug) : [...prev, slug]))

  const label = (slugs: string[]) =>
    slugs
      .map((s) => nameBySlug[s] ?? s)
      .sort()
      .join(', ') || 'none'
  const changed = [...draft].sort().join(' ') !== server.join(' ')

  const diff = useMemo<SectionDiffEntry[]>(
    () => (changed ? [{ label: 'Tags', before: label(server), after: label(draft) }] : []),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [changed, server, draft, nameBySlug],
  )
  useSaveSection({
    id: 'tags',
    title: 'Tags',
    diff,
    save: async () => {
      await update({ variables: { input: { id: pageId, tags: draft } } })
    },
  })

  async function onCreate() {
    const name = newName.trim()
    if (!name) return
    const res = await createTag({ variables: { name } })
    const slug = res.data?.createCMSTag.slug
    if (slug) setDraft((prev) => (prev.includes(slug) ? prev : [...prev, slug]))
    setNewName('')
  }

  // Selected slugs the library doesn't know still render so they can be removed
  const orphans = draft.filter((s) => !(s in nameBySlug))
  const selected = new Set(draft)

  return (
    <div className="grid gap-3">
      {library.length === 0 && orphans.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No tags in the library yet - create one below, or in CMS → Tags.
        </p>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {library.map((t) => {
            const on = selected.has(t.slug)
            return (
              <button
                key={t.slug}
                type="button"
                disabled={!canManage}
                onClick={() => toggle(t.slug)}
                className={cn(
                  'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs transition-colors',
                  on ? 'border-primary bg-primary/10 text-primary' : 'text-muted-foreground hover:bg-accent',
                )}
              >
                <TagIcon className="size-3" />
                {t.name}
              </button>
            )
          })}
          {orphans.map((s) => (
            <button
              key={s}
              type="button"
              disabled={!canManage}
              onClick={() => toggle(s)}
              title="Not in the tag library"
              className="inline-flex items-center gap-1 rounded-full border border-primary bg-primary/10 px-2.5 py-0.5 text-xs text-primary"
            >
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
          {changed && <span className="text-xs text-amber-600">unsaved</span>}
        </div>
      )}
    </div>
  )
}
