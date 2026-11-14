import { UploadCloudIcon, UploadIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'

export function EmptyWall({ filtered, path, canManage, onShowAll, onUpload }: { filtered: boolean; path: string; canManage: boolean; onShowAll: () => void; onUpload: () => void }) {
  return (
    <div className="grid min-h-[45vh] place-items-center rounded-xl border border-dashed">
      <div className="grid max-w-sm justify-items-center gap-2 p-8 text-center">
        <UploadCloudIcon className="size-10 text-muted-foreground" />
        <p className="text-base font-medium">{filtered ? 'Nothing matches' : path ? 'This folder is empty' : 'The library is empty'}</p>
        <p className="text-sm text-muted-foreground">
          {filtered ? 'Try other words, or show everything again.' : canManage ? 'Drop files here, paste a picture, or upload from your computer.' : 'Nothing has been uploaded yet.'}
        </p>
        {filtered ? (
          <Button type="button" size="sm" variant="outline" onClick={onShowAll}>
            Show everything
          </Button>
        ) : canManage ? (
          <Button type="button" size="sm" onClick={onUpload}>
            <UploadIcon className="size-4" />
            Upload
          </Button>
        ) : null}
      </div>
    </div>
  )
}
