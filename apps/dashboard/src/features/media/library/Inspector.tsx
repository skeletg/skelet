import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { CropIcon, Trash2Icon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { DeleteMediaAssetDocument, MoveMediaAssetDocument, type MediaAssetFieldsFragment as MediaAsset } from '../MediaLibrary.gql'
import { isEditable } from '../editor/output'
import { AssetPanel } from '../picker/AssetPanel'

// Radix refuses an empty value, and the library's root is the empty path
const ROOT = '__library__'

// The picked file beside the wall: its picture and words, then where it is kept. On a narrow page it lies over the wall
export function Inspector({
  asset,
  canManage,
  folders,
  onChanged,
  onDeleted,
  onEdit,
  onClose,
  onDirty,
  onError,
}: {
  asset: MediaAsset
  canManage: boolean
  folders: string[]
  onChanged: (a: MediaAsset) => void
  onDeleted: (a: MediaAsset) => void
  onEdit: () => void
  onClose: () => void
  onDirty: (dirty: boolean) => void
  onError: (message: string | null) => void
}) {
  return (
    <aside
      aria-label="Details"
      className="absolute inset-y-0 right-0 z-20 flex w-80 max-w-full shrink-0 flex-col border-l border-foreground/10 bg-background shadow-[0_0_40px_-10px_rgb(0_0_0/0.35)] @min-[72rem]/media:static @min-[72rem]/media:h-full @min-[72rem]/media:shadow-none"
    >
      <div className="flex h-[49px] shrink-0 items-center justify-between border-b border-foreground/10 pr-2 pl-4">
        <h2 className="min-w-0 truncate text-sm font-medium tracking-tight">{asset.title || asset.filename}</h2>
        <Button type="button" variant="ghost" size="icon" className="size-8 shrink-0 rounded-full text-muted-foreground" aria-label="Close details" onClick={onClose}>
          <XIcon className="size-4" />
        </Button>
      </div>
      <div className="grid flex-1 content-start gap-5 overflow-y-auto p-4">
        {canManage && isEditable(asset) ? (
          <Button type="button" variant="outline" size="sm" className="justify-between" onClick={onEdit}>
            <span className="flex items-center gap-2">
              <CropIcon className="size-4" />
              Edit picture
            </span>
            <span className="text-xs font-normal text-muted-foreground">crop, turn, colour</span>
          </Button>
        ) : null}
        <AssetPanel asset={asset} canManage={canManage} notes libraryLink={false} onChanged={onChanged} onDirty={onDirty} />
        {canManage ? <Filing key={asset.id} asset={asset} folders={folders} onChanged={onChanged} onDeleted={onDeleted} onError={onError} /> : null}
      </div>
    </aside>
  )
}

function Filing({
  asset,
  folders,
  onChanged,
  onDeleted,
  onError,
}: {
  asset: MediaAsset
  folders: string[]
  onChanged: (a: MediaAsset) => void
  onDeleted: (a: MediaAsset) => void
  onError: (message: string | null) => void
}) {
  const [filename, setFilename] = useState(asset.filename)
  const [folder, setFolder] = useState(asset.directoryPath)
  const [confirming, setConfirming] = useState(false)
  const [move, { loading: moving }] = useMutation(MoveMediaAssetDocument)
  const [remove] = useMutation(DeleteMediaAssetDocument)
  const renamed = filename.trim() !== asset.filename
  const refiled = folder !== asset.directoryPath
  const options = [...new Set(['', ...folders, asset.directoryPath])].sort()

  async function apply() {
    onError(null)
    try {
      const res = await move({ variables: { id: asset.id, targetPath: folder, filename: filename.trim() } })
      if (res.data?.moveMediaAsset) onChanged(res.data.moveMediaAsset)
    } catch (e) {
      onError(e instanceof Error ? e.message : "The file couldn't be moved.")
    }
  }

  async function onDelete() {
    onError(null)
    try {
      await remove({ variables: { id: asset.id, deleteFromStorage: true } })
      onDeleted(asset)
    } catch (e) {
      onError(e instanceof Error ? e.message : "The file couldn't be deleted.")
    }
  }

  return (
    <div className="grid gap-3 border-t border-foreground/10 pt-4">
      <div className="grid gap-1.5">
        <Label htmlFor="asset-filename">File name</Label>
        <Input id="asset-filename" value={filename} onChange={(e) => setFilename(e.target.value)} />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="asset-folder">Folder</Label>
        <Select value={folder || ROOT} onValueChange={(v) => setFolder(v === ROOT ? '' : v)}>
          <SelectTrigger id="asset-folder" size="sm" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options.map((f) => (
              <SelectItem key={f || ROOT} value={f || ROOT}>
                {f || 'Library'}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" size="sm" variant="outline" disabled={(!renamed && !refiled) || !filename.trim() || moving} onClick={() => void apply()}>
          {moving ? 'Moving…' : renamed && refiled ? 'Move and rename' : refiled ? 'Move' : 'Rename'}
        </Button>
        <Button type="button" size="sm" variant="ghost" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => setConfirming(true)}>
          <Trash2Icon className="size-4" />
          Delete
        </Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title="Delete this file?"
        description="It is removed from storage as well, so anything still using it will show a broken picture."
        confirmLabel="Delete file"
        destructive
        onConfirm={() => void onDelete()}
      />
    </div>
  )
}
