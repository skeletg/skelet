import { useState } from 'react'
import { AlertTriangleIcon, ExternalLinkIcon, FileTextIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { formatBytes } from '@/lib/format'
import type { MediaDirectoryQuery } from '../MediaLibrary.gql'

type UntrackedFile = MediaDirectoryQuery['mediaDirectory']['untrackedFiles'][number]

// Files sitting in this folder's storage with no record in the library - shown apart, since nothing can use them
export function Untracked({ files }: { files: UntrackedFile[] }) {
  const [open, setOpen] = useState<UntrackedFile | null>(null)
  if (files.length === 0) return null
  return (
    <section aria-label="Untracked files" className="grid gap-3">
      <h2 className="flex items-center gap-2 text-sm font-medium">
        <AlertTriangleIcon className="size-4 text-warning" />
        {files.length} in storage with no library record
      </h2>
      <ul className="grid grid-cols-[repeat(auto-fill,minmax(8rem,1fr))] gap-2">
        {files.map((file) => (
          <li key={file.objectKey}>
            <button
              type="button"
              onClick={() => setOpen(file)}
              title={file.filename}
              className="group relative block aspect-square w-full overflow-hidden rounded-lg border border-warning/40 bg-muted text-left outline-none focus-visible:ring-2 focus-visible:ring-warning"
            >
              {file.url && file.mimeType.startsWith('image/') ? (
                <img src={file.url} alt={file.filename} loading="lazy" className="size-full object-cover opacity-80" />
              ) : (
                <span className="grid size-full place-items-center text-muted-foreground">
                  <FileTextIcon className="size-8" />
                </span>
              )}
              <span className="absolute inset-x-0 bottom-0 truncate bg-background/85 px-2 py-1 text-[11px]">{file.filename}</span>
            </button>
          </li>
        ))}
      </ul>
      <Sheet open={!!open} onOpenChange={(o) => !o && setOpen(null)}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
          {open ? (
            <>
              <SheetHeader>
                <SheetTitle>{open.filename}</SheetTitle>
              </SheetHeader>
              <div className="grid gap-4 px-4">
                <div className="overflow-hidden rounded-lg border bg-muted">
                  {open.url && open.mimeType.startsWith('image/') ? (
                    <img src={open.url} alt={open.filename} className="max-h-80 w-full object-contain" />
                  ) : (
                    <div className="grid aspect-video place-items-center text-muted-foreground">
                      <FileTextIcon className="size-12" />
                    </div>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="outline" className="text-warning">
                    untracked
                  </Badge>
                  <Badge variant="outline">{open.mimeType}</Badge>
                  <span>{formatBytes(open.sizeBytes)}</span>
                  {open.url ? (
                    <Button variant="ghost" size="xs" asChild>
                      <a href={open.url} target="_blank" rel="noreferrer">
                        <ExternalLinkIcon className="size-3" />
                        Open
                      </a>
                    </Button>
                  ) : null}
                </div>
                <div className="grid gap-1 text-sm">
                  <p className="text-xs font-medium text-muted-foreground">Storage key</p>
                  <p className="font-mono text-xs break-all">{open.objectKey}</p>
                </div>
                <div className="grid gap-1 text-sm">
                  <p className="text-xs font-medium text-muted-foreground">Last modified</p>
                  <p className="text-xs">{new Date(open.lastModified).toLocaleString()}</p>
                </div>
                <Alert>
                  <AlertTriangleIcon className="size-4" />
                  <AlertTitle>No library record</AlertTitle>
                  <AlertDescription>This file exists in storage but is not tracked in the media library database. It may be an orphaned upload or was added directly to storage.</AlertDescription>
                </Alert>
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </section>
  )
}
