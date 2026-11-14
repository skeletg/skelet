import { XIcon } from 'lucide-react'
import type { UploadItem } from '../picker/useUploadQueue'

export function UploadList({ items, onDismiss, onClear }: { items: UploadItem[]; onDismiss: (key: string) => void; onClear: () => void }) {
  if (items.length === 0) return null
  const finished = items.filter((u) => u.status !== 'uploading').length
  return (
    <ul aria-label="Uploads" className="grid gap-1.5 rounded-lg border bg-muted/40 px-3 py-2.5">
      {items.map((u) => (
        <li key={u.key} className="flex items-center gap-3 text-xs">
          <span className="min-w-0 flex-1 truncate">{u.name}</span>
          {u.status === 'uploading' ? (
            <span className="h-1.5 w-40 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={Math.round(u.progress * 100)} aria-label={`Uploading ${u.name}`}>
              <span className="block h-full bg-foreground transition-[width]" style={{ width: `${Math.max(4, u.progress * 100)}%` }} />
            </span>
          ) : u.status === 'done' ? (
            <span className="text-success">Added</span>
          ) : (
            <span className="text-destructive">{u.error}</span>
          )}
          {u.status !== 'uploading' ? (
            <button type="button" aria-label={`Dismiss ${u.name}`} onClick={() => onDismiss(u.key)} className="text-muted-foreground hover:text-foreground">
              <XIcon className="size-3.5" />
            </button>
          ) : null}
        </li>
      ))}
      {finished > 1 ? (
        <li className="flex justify-end">
          <button type="button" onClick={onClear} className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline">
            Clear the {finished} finished
          </button>
        </li>
      ) : null}
    </ul>
  )
}
