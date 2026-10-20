import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { ExternalLinkIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  CreateUrlRewriteDocument,
  DeleteUrlRewriteDocument,
  UpdateUrlRewriteDocument,
  UrlRewritesDocument,
  type UrlRewritesQuery,
} from './Catalog.gql'

type UrlRewrite = UrlRewritesQuery['urlRewrites']['items'][number]

const REDIRECT_LABELS: Record<number, string> = {
  0: 'Canonical (current URL)',
  301: 'Permanent redirect',
  302: 'Temporary redirect',
}

function RedirectTypeBadge({ type }: { type: number }) {
  if (type === 0)
    return <Badge variant="secondary" className="shrink-0 font-normal">Canonical</Badge>
  if (type === 301)
    return <Badge variant="outline" className="shrink-0 font-normal">301 Permanent</Badge>
  return <Badge variant="outline" className="shrink-0 font-normal">302 Temporary</Badge>
}

export function UrlRewritesSection({
  entityId,
  entityType = 'PRODUCT',
  entityLabel = 'product',
  canManage,
}: {
  entityId: string
  entityType?: 'PRODUCT' | 'COLLECTION'
  entityLabel?: string
  canManage: boolean
}) {
  const rewritesVariables = { entityType: entityType as never, entityId }
  const { data, loading, error } = useQuery(UrlRewritesDocument, { variables: rewritesVariables })
  const rewrites = data?.urlRewrites.items ?? []

  const refetchQueries = [{ query: UrlRewritesDocument, variables: rewritesVariables }]
  const [createRewrite, { loading: creating }] = useMutation(CreateUrlRewriteDocument, { refetchQueries })
  const [updateRewrite, { loading: updating }] = useMutation(UpdateUrlRewriteDocument, { refetchQueries })
  const [deleteRewrite, { loading: deleting }] = useMutation(DeleteUrlRewriteDocument, { refetchQueries })

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<UrlRewrite | null>(null)
  const [toDelete, setToDelete] = useState<UrlRewrite | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)

  function openCreate() {
    setEditing(null)
    setMutationError(null)
    setFormOpen(true)
  }

  function openEdit(rewrite: UrlRewrite) {
    setEditing(rewrite)
    setMutationError(null)
    setFormOpen(true)
  }

  async function handleFormSubmit(values: { requestPath: string; redirectType: number; description: string }) {
    setMutationError(null)
    try {
      if (editing) {
        await updateRewrite({
          variables: {
            input: {
              id: editing.id,
              requestPath: values.requestPath,
              redirectType: values.redirectType,
              description: values.description || undefined,
            },
          },
        })
      } else {
        await createRewrite({
          variables: {
            input: {
              entityType: entityType as never,
              entityId,
              requestPath: values.requestPath,
              redirectType: values.redirectType,
              description: values.description || undefined,
            },
          },
        })
      }
      setFormOpen(false)
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function handleDelete() {
    if (!toDelete) return
    setMutationError(null)
    try {
      await deleteRewrite({ variables: { id: toDelete.id } })
      setToDelete(null)
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const canonical = rewrites.find((r) => r.redirectType === 0)
  const redirects = rewrites.filter((r) => r.redirectType !== 0)

  if (loading) return <p className="rounded-md border bg-muted/20 p-4 text-sm text-muted-foreground">Loading rewrites…</p>
  if (error) return (
    <Alert variant="destructive">
      <AlertTitle>Couldn't load URL rewrites</AlertTitle>
      <AlertDescription>{error.message}</AlertDescription>
    </Alert>
  )

  return (
    <div className="grid gap-5">
      {canonical && (
        <div className="rounded-md border bg-muted/25 p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="grid min-w-0 gap-2">
              <div className="grid gap-1">
                <p className="text-sm font-medium">
                  Canonical {entityLabel} path
                </p>
                <p className="text-sm text-muted-foreground">
                  This is the current public URL generated for the {entityLabel}.
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <code className="rounded bg-background px-2 py-1 font-mono text-sm break-all">{canonical.requestPath}</code>
                <ExternalLinkIcon className="size-4 shrink-0 text-muted-foreground" />
                <code className="rounded bg-background px-2 py-1 font-mono text-sm break-all text-muted-foreground">{canonical.targetPath}</code>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <RedirectTypeBadge type={canonical.redirectType} />
            </div>
          </div>
        </div>
      )}

      {redirects.length > 0 && (
        <div className="grid gap-3">
          <div className="grid gap-1">
            <p className="text-sm font-medium">Redirects into this {entityLabel}</p>
            <p className="text-sm text-muted-foreground">
              Use these for old slugs, campaign URLs, and renamed {entityLabel}s. Customer
              requests to these paths will resolve to the {entityLabel} target path below.
            </p>
          </div>
          {redirects.map((rewrite) => (
            <div key={rewrite.id} className="flex flex-wrap items-start justify-between gap-3 rounded-md border p-4">
              <div className="grid min-w-0 gap-2">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <code className="rounded bg-muted/40 px-2 py-1 font-mono text-sm break-all">{rewrite.requestPath}</code>
                  <span className="text-muted-foreground">→</span>
                  <code className="rounded bg-muted/40 px-2 py-1 font-mono text-sm break-all text-muted-foreground">{rewrite.targetPath}</code>
                </div>
                {rewrite.description && (
                  <p className="text-sm text-muted-foreground">{rewrite.description}</p>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <RedirectTypeBadge type={rewrite.redirectType} />
                {rewrite.isAutogenerated && (
                  <Badge variant="secondary" className="shrink-0 font-normal text-xs">System</Badge>
                )}
                {canManage && !rewrite.isAutogenerated && (
                  <>
                    <Button aria-label="Edit URL rewrite"
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      onClick={() => openEdit(rewrite)}
                    >
                      <PencilIcon className="size-3.5" />
                    </Button>
                    <Button aria-label="Delete URL rewrite"
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7 text-destructive hover:text-destructive"
                      onClick={() => { setMutationError(null); setToDelete(rewrite) }}
                    >
                      <Trash2Icon className="size-3.5" />
                    </Button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {redirects.length === 0 && !canonical && (
        <div className="rounded-md border border-dashed bg-muted/20 p-5">
          <p className="text-sm font-medium">No URL rewrites configured.</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Add a redirect when an old {entityLabel} URL, imported catalog path, or campaign path
            should point to this {entityLabel}.
          </p>
        </div>
      )}

      {mutationError && (
        <Alert variant="destructive">
          <AlertTitle>Action failed</AlertTitle>
          <AlertDescription>{mutationError}</AlertDescription>
        </Alert>
      )}

      {canManage && (
        <div>
          <Button type="button" variant="outline" onClick={openCreate}>
            <PlusIcon className="size-4" />
            Add redirect
          </Button>
        </div>
      )}

      <UrlRewriteFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        initial={editing}
        saving={creating || updating}
        error={mutationError}
        onSubmit={handleFormSubmit}
      />

      <AlertDialog open={Boolean(toDelete)} onOpenChange={(v) => { if (!v) setToDelete(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete redirect?</AlertDialogTitle>
            <AlertDialogDescription>
              Permanently removes the redirect for{' '}
              <code className="font-mono text-xs">{toDelete?.requestPath}</code>.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => { e.preventDefault(); handleDelete() }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete redirect'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function UrlRewriteFormDialog({
  open,
  onOpenChange,
  initial,
  saving,
  error,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initial: UrlRewrite | null
  saving: boolean
  error: string | null
  onSubmit: (values: { requestPath: string; redirectType: number; description: string }) => Promise<void>
}) {
  const [requestPath, setRequestPath] = useState(initial?.requestPath ?? '')
  const [redirectType, setRedirectType] = useState(String(initial?.redirectType ?? 301))
  const [description, setDescription] = useState(initial?.description ?? '')

  // Sync when dialog opens with a different row
  const key = initial?.id ?? 'new'

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    await onSubmit({
      requestPath: requestPath.trim(),
      redirectType: Number(redirectType),
      description,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange} key={key}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{initial ? 'Edit redirect' : 'Add redirect'}</DialogTitle>
        </DialogHeader>
        <form id="url-rewrite-form" onSubmit={handleSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="rw-path">Request path</Label>
            <Input
              id="rw-path"
              required
              value={requestPath}
              onChange={(e) => setRequestPath(e.target.value)}
              placeholder="old-product-url"
              disabled={saving}
            />
            <p className="text-xs text-muted-foreground">
              URL path to redirect - just the slug segment, no leading slash needed.
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="rw-type">Redirect type</Label>
            <Select value={redirectType} onValueChange={setRedirectType} disabled={saving}>
              <SelectTrigger id="rw-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="301">{REDIRECT_LABELS[301]}</SelectItem>
                <SelectItem value="302">{REDIRECT_LABELS[302]}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="rw-desc">Description <span className="text-muted-foreground font-normal">(optional)</span></Label>
            <Input
              id="rw-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Old URL from your previous site"
              disabled={saving}
            />
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Save failed</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </form>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            Cancel
          </Button>
          <Button type="submit" form="url-rewrite-form" disabled={saving}>
            {saving ? 'Saving…' : initial ? 'Save changes' : 'Add redirect'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
