import { useState, type FormEvent } from 'react'
import { useMutation } from '@apollo/client/react'
import { DownloadIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
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
  AddProductDownloadDocument,
  ProductDocument,
  RemoveProductDownloadDocument,
  UpdateProductDownloadDocument,
  type ProductDownloadFieldsFragment,
} from './Catalog.gql'

type WithDownloads = {
  id: string
  downloads: ProductDownloadFieldsFragment[]
}

type FormValues = {
  title: string
  url: string
  maxDownloads: string
  sortOrder: string
}

export function ProductDownloadsSection({
  product,
  canManage,
}: {
  product: WithDownloads
  canManage: boolean
}) {
  const downloads = [...product.downloads].sort((a, b) => a.sortOrder - b.sortOrder)

  const refetchQueries = [{ query: ProductDocument, variables: { id: product.id } }]
  const [addDownload, { loading: creating }] = useMutation(AddProductDownloadDocument, { refetchQueries })
  const [updateDownload, { loading: updating }] = useMutation(UpdateProductDownloadDocument, { refetchQueries })
  const [removeDownload, { loading: deleting }] = useMutation(RemoveProductDownloadDocument, { refetchQueries })

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ProductDownloadFieldsFragment | null>(null)
  const [toDelete, setToDelete] = useState<ProductDownloadFieldsFragment | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)

  function openCreate() {
    setEditing(null)
    setMutationError(null)
    setFormOpen(true)
  }

  function openEdit(d: ProductDownloadFieldsFragment) {
    setEditing(d)
    setMutationError(null)
    setFormOpen(true)
  }

  async function handleFormSubmit(values: FormValues) {
    setMutationError(null)
    const maxDownloads = values.maxDownloads.trim() === '' ? null : Number(values.maxDownloads)
    const sortOrder = values.sortOrder.trim() === '' ? undefined : Number(values.sortOrder)
    try {
      if (editing) {
        await updateDownload({
          variables: {
            input: {
              id: editing.id,
              title: values.title.trim(),
              url: values.url.trim(),
              maxDownloads,
              sortOrder,
            },
          },
        })
      } else {
        await addDownload({
          variables: {
            input: {
              productId: product.id,
              title: values.title.trim(),
              url: values.url.trim(),
              maxDownloads,
              sortOrder,
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
      await removeDownload({ variables: { id: toDelete.id } })
      setToDelete(null)
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="grid gap-5">
      <div className="grid gap-1">
        <p className="text-sm text-muted-foreground">
          Files this product delivers. Buying the product grants the customer token-gated access to
          each download; the link expires after the download cap (if set) is reached.
        </p>
      </div>

      {downloads.length === 0 ? (
        <div className="rounded-md border border-dashed bg-muted/20 p-5">
          <p className="text-sm font-medium">No downloads attached.</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Add a file URL to deliver this product digitally on purchase.
          </p>
        </div>
      ) : (
        <div className="grid gap-3">
          {downloads.map((d) => (
            <div key={d.id} className="flex flex-wrap items-start justify-between gap-3 rounded-md border p-4">
              <div className="grid min-w-0 gap-2">
                <div className="flex items-center gap-2">
                  <DownloadIcon className="size-4 shrink-0 text-muted-foreground" />
                  <p className="text-sm font-medium break-all">{d.title}</p>
                </div>
                <code className="rounded bg-muted/40 px-2 py-1 font-mono text-xs break-all text-muted-foreground">
                  {d.url}
                </code>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Badge variant="outline" className="shrink-0 font-normal">
                  {d.maxDownloads == null ? 'Unlimited' : `${d.maxDownloads} download${d.maxDownloads === 1 ? '' : 's'}`}
                </Badge>
                {canManage && (
                  <>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      onClick={() => openEdit(d)}
                    >
                      <PencilIcon className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7 text-destructive hover:text-destructive"
                      onClick={() => { setMutationError(null); setToDelete(d) }}
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
            Add download
          </Button>
        </div>
      )}

      <ProductDownloadFormDialog
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
            <AlertDialogTitle>Remove download?</AlertDialogTitle>
            <AlertDialogDescription>
              Removes <span className="font-medium">{toDelete?.title}</span> from this product.
              Existing grants already issued to customers keep working.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => { e.preventDefault(); handleDelete() }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Removing…' : 'Remove download'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ProductDownloadFormDialog({
  open,
  onOpenChange,
  initial,
  saving,
  error,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initial: ProductDownloadFieldsFragment | null
  saving: boolean
  error: string | null
  onSubmit: (values: FormValues) => Promise<void>
}) {
  const [title, setTitle] = useState(initial?.title ?? '')
  const [url, setUrl] = useState(initial?.url ?? '')
  const [maxDownloads, setMaxDownloads] = useState(
    initial?.maxDownloads == null ? '' : String(initial.maxDownloads),
  )
  const [sortOrder, setSortOrder] = useState(String(initial?.sortOrder ?? 0))

  // Remount on a different row so the controlled inputs reset
  const key = initial?.id ?? 'new'

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    await onSubmit({ title, url, maxDownloads, sortOrder })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange} key={key}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{initial ? 'Edit download' : 'Add download'}</DialogTitle>
        </DialogHeader>
        <form id="product-download-form" onSubmit={handleSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="dl-title">Title</Label>
            <Input
              id="dl-title"
              required
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="User manual (PDF)"
              disabled={saving}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="dl-url">File URL</Label>
            <Input
              id="dl-url"
              required
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://cdn.example.com/files/manual.pdf"
              disabled={saving}
            />
            <p className="text-xs text-muted-foreground">
              Where the file lives. The customer never sees this directly - they get a token link
              that redirects here.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-2">
              <Label htmlFor="dl-max">
                Download cap <span className="text-muted-foreground font-normal">(optional)</span>
              </Label>
              <Input
                id="dl-max"
                type="number"
                min={1}
                value={maxDownloads}
                onChange={(e) => setMaxDownloads(e.target.value)}
                placeholder="Unlimited"
                disabled={saving}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="dl-sort">Sort order</Label>
              <Input
                id="dl-sort"
                type="number"
                value={sortOrder}
                onChange={(e) => setSortOrder(e.target.value)}
                disabled={saving}
              />
            </div>
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
          <Button type="submit" form="product-download-form" disabled={saving}>
            {saving ? 'Saving…' : initial ? 'Save changes' : 'Add download'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
