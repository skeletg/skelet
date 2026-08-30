import { useMemo, useState } from 'react'
import { useDraft } from '@/shared/saving/useDraft'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadingState } from '@/components/ui/loading-state'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CmsSnippetsDocument,
  CreateCmsSnippetDocument,
  UpdateCmsSnippetDocument,
  DeleteCmsSnippetDocument,
  type CmsSnippetsQuery,
} from './Cms.gql'
import { PageBlockCanvas } from './PageBlockCanvas'
import { parseDocument, serializeDocument, type CmsBlock } from './page-blocks'

type Snippet = CmsSnippetsQuery['cmsSnippets'][number]

// CmsSnippetsPage - the Section library - A section is a reusable block subtree; pages embed it via the "Reusable section" (ref) block,
// expanded server-side so one edit updates every page - List on the left, block editor for the selected section on the right
export function CmsSnippetsPage() {
  const { has } = usePermissions()
  const canManage = has('cms.manage')
  const { data, loading } = useQuery(CmsSnippetsDocument)
  const snippets = useMemo(() => data?.cmsSnippets ?? [], [data])
  const refetch = [{ query: CmsSnippetsDocument }]
  const [createSnippet] = useMutation(CreateCmsSnippetDocument, { refetchQueries: refetch })
  const [updateSnippet] = useMutation(UpdateCmsSnippetDocument, { refetchQueries: refetch })
  const [deleteSnippet] = useMutation(DeleteCmsSnippetDocument, { refetchQueries: refetch })

  const [editing, setEditing] = useState<Snippet | 'new' | null>(null)

  if (loading) return <LoadingState label="Loading sections…" />

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/cms"
        backLabel="Back to CMS"
        title="Sections"
        meta={<span className="text-xs text-muted-foreground">Reusable block sections - edit once, update everywhere.</span>}
        actions={
          canManage && (
            <Button size="sm" onClick={() => setEditing('new')}>
              <PlusIcon className="size-4" /> New section
            </Button>
          )
        }
      />
      <DetailPageBody>
        <DetailPageSection title="Library">
          {snippets.length === 0 ? (
            <p className="text-sm text-muted-foreground">No reusable sections yet.</p>
          ) : (
            <ul className="grid gap-2">
              {snippets.map((s) => (
                <li key={s.id} className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
                  <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setEditing(s)}>
                    <span className="block text-sm font-medium">{s.name}</span>
                    <code className="text-xs text-muted-foreground">{s.slug}</code>
                  </button>
                  {canManage && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-destructive"
                      onClick={() => deleteSnippet({ variables: { id: s.id } })}
                      aria-label={`Delete ${s.name}`}
                    >
                      <Trash2Icon className="size-4" />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </DetailPageSection>

        {editing && (
          <SnippetEditor
            key={editing === 'new' ? 'new' : editing.id}
            snippet={editing === 'new' ? null : editing}
            canManage={canManage}
            onClose={() => setEditing(null)}
            onCreate={async (slug, name, blocks) => {
              await createSnippet({ variables: { input: { slug, name, blocks: serializeDocument(blocks) } } })
              setEditing(null)
            }}
            onUpdate={async (id, name, blocks) => {
              await updateSnippet({ variables: { input: { id, name, blocks: serializeDocument(blocks) } } })
              setEditing(null)
            }}
          />
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function SnippetEditor({
  snippet,
  canManage,
  onClose,
  onCreate,
  onUpdate,
}: {
  snippet: Snippet | null
  canManage: boolean
  onClose: () => void
  onCreate: (slug: string, name: string, blocks: CmsBlock[]) => Promise<void>
  onUpdate: (id: string, name: string, blocks: CmsBlock[]) => Promise<void>
}) {
  const draft = useDraft({
    name: snippet?.name ?? '',
    slug: snippet?.slug ?? '',
    blocks: parseDocument(snippet?.blocks)?.blocks ?? [] as CmsBlock[],
  })
  const { name, slug, blocks } = draft.values
  const { setName, setSlug, setBlocks } = draft.setters
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function save() {
    setError(null)
    setSaving(true)
    try {
      if (snippet) await onUpdate(snippet.id, name.trim(), blocks)
      else await onCreate(slug.trim(), name.trim(), blocks)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <DetailPageSection title={snippet ? `Edit “${snippet.name}”` : 'New section'}>
      <div className="grid gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="snip-name">Name</Label>
            <Input id="snip-name" value={name} onChange={(e) => setName(e.target.value)} disabled={!canManage || saving} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="snip-slug">Slug</Label>
            <Input id="snip-slug" value={slug} onChange={(e) => setSlug(e.target.value)} disabled={!canManage || saving || !!snippet} placeholder="promo-banner" />
          </div>
        </div>
        <div className="grid gap-1.5">
          <Label>Blocks</Label>
          <PageBlockCanvas blocks={blocks} onChange={setBlocks} disabled={!canManage} />
        </div>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Save failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose} disabled={saving}>Cancel</Button>
          {canManage && (
            <Button onClick={save} disabled={saving || !name.trim() || (!snippet && !slug.trim())}>
              {saving ? 'Saving…' : 'Save section'}
            </Button>
          )}
        </div>
      </div>
    </DetailPageSection>
  )
}
