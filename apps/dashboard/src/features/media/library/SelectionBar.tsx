import { FolderInputIcon, Trash2Icon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

export function SelectionBar({
  count,
  folders,
  busy,
  onMove,
  onDelete,
  onClear,
}: {
  count: number
  folders: string[]
  busy: boolean
  onMove: (folder: string) => void
  onDelete: () => void
  onClear: () => void
}) {
  return (
    <div className="pointer-events-none sticky bottom-4 z-10 mt-auto flex justify-center px-4 pt-4">
      <div role="toolbar" aria-label="Picked files" className="pointer-events-auto flex items-center gap-1 rounded-full border border-foreground/10 bg-background/95 py-1 pr-1 pl-4 text-sm shadow-[0_1px_2px_rgb(0_0_0/0.06),0_12px_32px_-12px_rgb(0_0_0/0.3)] backdrop-blur-md">
        <span className="mr-2 font-medium tabular-nums">{count} picked</span>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="sm" className="rounded-full" disabled={busy}>
              <FolderInputIcon className="size-4" />
              Move to
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="center" className="max-h-72 w-64 overflow-y-auto">
            <DropdownMenuLabel>Move {count} files to</DropdownMenuLabel>
            {['', ...folders].map((f) => (
              <DropdownMenuItem key={f || 'library'} onSelect={() => onMove(f)}>
                {f || 'Library'}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button type="button" variant="ghost" size="sm" className="rounded-full text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={onDelete}>
          <Trash2Icon className="size-4" />
          Delete
        </Button>
        <Button type="button" variant="ghost" size="icon" className="size-8 rounded-full text-muted-foreground" aria-label="Clear the picks" onClick={onClear}>
          <XIcon className="size-4" />
        </Button>
      </div>
    </div>
  )
}
