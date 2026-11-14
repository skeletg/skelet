import { FolderPlusIcon, LockIcon, MoreHorizontalIcon, PencilIcon, Trash2Icon, UploadIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { MediaVisibility } from '@/graphql/schema.types'
import { cn } from '@/lib/utils'
import { buildBreadcrumbs } from '../directory-paths'
import { AreaSwitch } from './AreaSwitch'

// Where the wall stands in the library, with what can be done to that folder and the way to add files to it
export function PathBar({
  path,
  canManage,
  uploading,
  area,
  onArea,
  onNavigate,
  onRename,
  onDelete,
  onNewFolder,
  onUpload,
}: {
  path: string
  canManage: boolean
  uploading: boolean
  area: MediaVisibility
  onArea?: (area: MediaVisibility) => void
  onNavigate: (path: string) => void
  onRename: () => void
  onDelete: (recursive: boolean) => void
  onNewFolder: () => void
  onUpload: () => void
}) {
  const breadcrumbs = buildBreadcrumbs(path)
  if (area === 'PRIVATE') breadcrumbs[0] = { ...breadcrumbs[0]!, name: 'Private files' }
  return (
    <div className="sticky top-0 z-10 border-b border-foreground/10 bg-background/85 backdrop-blur-md">
      <div className="flex min-h-[49px] flex-wrap items-center justify-between gap-3 px-4 py-2">
        {onArea ? <AreaSwitch area={area} onArea={onArea} className="@min-[56rem]/media:hidden" /> : null}
        <nav aria-label="Folder" className="flex min-w-0 flex-wrap items-center gap-0.5 text-sm">
          {area === 'PRIVATE' ? <LockIcon aria-hidden className="mr-1 size-3.5 text-muted-foreground" /> : null}
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
