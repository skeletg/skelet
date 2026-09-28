import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { LoadingState } from '@/components/ui/loading-state'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { sameValue } from '@/lib/form-drafts'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CmsSnippetsDocument,
  CreateCmsSnippetDocument,
  UpdateCmsSnippetDocument,
  DeleteCmsSnippetDocument,
  type CmsSnippetsQuery,
} from './Cms.gql'
import { PageBlockCanvas } from './PageBlockCanvas'
import type { CmsBlock, CmsDocument } from './page-blocks'
import { snippetSchema, toSnippetBlocks, toSnippetValues } from './schemas/snippet'

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
              await createSnippet({ variables: { input: { slug, name, blocks } } })
              setEditing(null)
            }}
            onUpdate={async (id, name, blocks) => {
              await updateSnippet({ variables: { input: { id, name, blocks } } })
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
  onCreate: (slug: string, name: string, blocks: CmsDocument) => Promise<void>
  onUpdate: (id: string, name: string, blocks: CmsDocument) => Promise<void>
}) {
  // Keyed on the snippet by the page, so each open starts from the stored section
  const [baseline] = useState(() => toSnippetValues(snippet))
  const { form, error, onSubmit } = useDialogForm({
    baseline,
    schema: snippetSchema,
    submit: async (v) => {
      if (snippet) await onUpdate(snippet.id, v.name.trim(), toSnippetBlocks(v))
      else await onCreate(v.slug.trim(), v.name.trim(), toSnippetBlocks(v))
    },
  })
  const dirty = useStore(form.store, (s) => !sameValue(s.values, baseline))
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  useUnsavedChangesGuard(dirty, snippet ? `Section · ${snippet.name}` : 'New section')

  return (
    <DetailPageSection title={snippet ? `Edit “${snippet.name}”` : 'New section'}>
      <form className="grid gap-4" onSubmit={onSubmit}>
        <fieldset disabled={!canManage || submitting} className="grid gap-4 sm:grid-cols-2">
          <form.AppField name="name">{(f) => <f.TextField id="snip-name" label="Name" />}</form.AppField>
          <form.AppField name="slug">
            {(f) => <f.TextField id="snip-slug" label="Slug" placeholder="promo-banner" disabled={!!snippet} />}
          </form.AppField>
        </fieldset>
        <div className="grid gap-1.5">
          <Label>Blocks</Label>
          <form.Field name="blocks">
            {(f) => (
              <PageBlockCanvas blocks={f.state.value as CmsBlock[]} onChange={(blocks) => f.handleChange(blocks)} disabled={!canManage} />
            )}
          </form.Field>
        </div>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Save failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose} disabled={submitting}>Cancel</Button>
          {canManage && (
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Saving…' : 'Save section'}
            </Button>
          )}
        </div>
      </form>
    </DetailPageSection>
  )
}
