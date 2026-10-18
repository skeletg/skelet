import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, TagIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { LoadingState } from '@/components/ui/loading-state'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import { changed } from '@/shared/saving/diff'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { useSaveRegistry, useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  CmsTagsDocument,
  CreateCmsTagDocument,
  RenameCmsTagDocument,
  DeleteCmsTagDocument,
  type CmsTagsQuery,
} from './Cms.gql'

type Tag = CmsTagsQuery['cmsTags'][number]

// CmsTagsPage - the managed tag vocabulary - Create tags here, then assign them
// to pages from the page editor's Tags picker - Slug is the stable key pages
// store; renaming changes only the display name (assignments survive) - Deleting
// a tag also strips it from every page that carries it
export function CmsTagsPage() {
  const { has } = usePermissions()
  const canManage = has('cms.manage')
  const { data, loading } = useQuery(CmsTagsDocument)
  const tags = useMemo(() => data?.cmsTags ?? [], [data])
  const refetch = [{ query: CmsTagsDocument }]
  const [createTag] = useMutation(CreateCmsTagDocument, { refetchQueries: refetch })
  const [renameTag] = useMutation(RenameCmsTagDocument, { refetchQueries: refetch })
  const [deleteTag] = useMutation(DeleteCmsTagDocument, { refetchQueries: refetch })

  const [newName, setNewName] = useState('')
  const [pendingDelete, setPendingDelete] = useState<Tag | null>(null)

  // Renames stage and go through the header Save like any other field edit - they used to write on blur, with no
  // review and no error, so a failed rename still showed the new name. Create and delete keep their own buttons
  const registry = useSaveRegistry('Tags')
  const [renames, setRenames] = useState<Map<string, string>>(new Map())
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const diff = tags
    .map((t) => (renames.has(t.slug) ? changed(`Tag · ${t.slug} · name`, t.name, renames.get(t.slug)?.trim()) : null))
    .filter((e): e is SectionDiffEntry => e !== null)
  useSaveSection(
    {
      id: 'tags',
      title: 'Tags',
      diff,
      save: async () => {
        for (const t of tags) {
          const name = renames.get(t.slug)?.trim()
          if (name && name !== t.name) await renameTag({ variables: { slug: t.slug, name } })
        }
        setRenames(new Map())
      },
    },
    registry.contextValue,
  )

  async function onConfirmSave() {
    setSaving(true)
    setSaveError(null)
    try {
      await registry.saveAll()
      setConfirmingSave(false)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Could not save the tags')
    } finally {
      setSaving(false)
    }
  }

  async function onCreate() {
    const name = newName.trim()
    if (!name) return
    await createTag({ variables: { name } })
    setNewName('')
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/cms/pages"
        backLabel="Back to pages"
        title="Tags"
        meta={<span>Manage the shared tag vocabulary used across CMS pages.</span>}
        actions={
          canManage && (
            <Button size="sm" disabled={!registry.isDirty || saving} onClick={() => setConfirmingSave(true)}>
              Save
            </Button>
          )
        }
      />
      <DetailPageBody>
        {canManage && (
          <DetailPageSection title="New tag" description="The slug is derived from the name automatically.">
            <div className="flex max-w-md gap-2">
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    void onCreate()
                  }
                }}
                placeholder="e.g. Summer Sale"
              />
              <Button className="h-10" type="button" onClick={() => void onCreate()} disabled={!newName.trim()}>
                <PlusIcon className="size-4" /> Add
              </Button>
            </div>
          </DetailPageSection>
        )}

        <DetailPageSection title={`All tags (${tags.length})`}>
          {loading ? (
            <LoadingState />
          ) : tags.length === 0 ? (
            <p className="text-sm text-muted-foreground">No tags yet. Create one above.</p>
          ) : (
            <ul className="grid gap-1.5">
              {tags.map((t) => (
                <li key={t.slug} className="flex items-center gap-3 rounded-md border p-2">
                  <TagIcon className="size-4 shrink-0 text-muted-foreground" />
                  <Input
                    aria-label={`Name of tag ${t.slug}`}
                    value={renames.get(t.slug) ?? t.name}
                    disabled={!canManage}
                    className="h-8 max-w-xs"
                    onChange={(e) => {
                      const name = e.target.value
                      setRenames((prev) => new Map(prev).set(t.slug, name))
                    }}
                  />
                  <code className="font-mono text-xs text-muted-foreground">{t.slug}</code>
                  <Badge variant="secondary" className="ml-auto">
                    {t.pageCount} {t.pageCount === 1 ? 'page' : 'pages'}
                  </Badge>
                  {canManage && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-muted-foreground hover:text-destructive"
                      onClick={() => setPendingDelete(t)}
                      aria-label={`Delete ${t.name}`}
                    >
                      <Trash2Icon className="size-4" />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </DetailPageSection>
      </DetailPageBody>

      <AlertDialog open={!!pendingDelete} onOpenChange={(o) => !o && setPendingDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete tag “{pendingDelete?.name}”?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the tag from the library and from the{' '}
              {pendingDelete?.pageCount ?? 0} page(s) that use it. This can't be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => {
                if (pendingDelete) void deleteTag({ variables: { slug: pendingDelete.slug } })
                setPendingDelete(null)
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel="Tags"
        sections={registry.dirtySections}
        saving={saving}
        error={saveError}
        onCancel={() => setConfirmingSave(false)}
        onConfirm={() => void onConfirmSave()}
      />
    </DetailPageLayout>
  )
}
