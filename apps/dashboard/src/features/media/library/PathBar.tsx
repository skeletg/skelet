import { FolderPlusIcon, MoreHorizontalIcon, PencilIcon, Trash2Icon, UploadIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { buildBreadcrumbs } from '../directory-paths'

// Where the wall stands in the library, with what can be done to that folder and the way to add files to it
export function PathBar({
  path,
  canManage,
  uploading,
  onNavigate,
  onRename,
  onDelete,
  onNewFolder,
  onUpload,
}: {
  path: string
  canManage: boolean
  uploading: boolean
  onNavigate: (path: string) => void
  onRename: () => void
  onDelete: (recursive: boolean) => void
  onNewFolder: () => void
  onUpload: () => void
}) {
  const breadcrumbs = buildBreadcrumbs(path)
  return (
    <div className="sticky top-0 z-10 border-b border-foreground/10 bg-background/85 backdrop-blur-md">
      <div className="flex min-h-[49px] flex-wrap items-center justify-between gap-3 px-4 py-2">
        <nav aria-label="Folder" className="flex min-w-0 flex-wrap items-center gap-0.5 text-sm">
          {breadcrumbs.map((crumb, i) => {
            const here = i === breadcrumbs.length - 1
            return (
              <span key={crumb.path || 'root'} className="inline-flex items-center gap-0.5">
                {i > 0 ? <span className="text-muted-foreground/60">/</span> : null}
                <button
                  type="button"
                  aria-current={here ? 'page' : undefined}
                  onClick={() => onNavigate(crumb.path)}
                  className={cn('rounded-md px-1.5 py-0.5 transition-colors hover:bg-muted', here ? 'font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}
                >
                  {crumb.name}
                </button>
              </span>
            )
          })}
        </nav>
        {canManage ? (
          <div className="flex shrink-0 items-center gap-2">
            {path ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="sm">
                    <MoreHorizontalIcon className="size-4" />
                    Folder
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={onRename}>
                    <PencilIcon className="size-4" />
                    Rename
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onSelect={() => onDelete(false)}>
                    <Trash2Icon className="size-4" />
                    Delete if empty
                  </DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onSelect={() => onDelete(true)}>
                    <Trash2Icon className="size-4" />
                    Delete with everything in it
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
            <Button variant="outline" size="sm" className="@min-[56rem]/media:hidden" onClick={onNewFolder}>
              <FolderPlusIcon className="size-4" />
              New folder
            </Button>
            <Button size="sm" onClick={onUpload} disabled={uploading}>
              <UploadIcon className="size-4" />
              Upload
            </Button>
          </div>
        ) : null}
      </div>
    </div>
  )
}
