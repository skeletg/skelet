import { FolderPlusIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { MediaVisibility } from '@/graphql/schema.types'
import type { DirTreeNode } from '../directory-paths'
import { DirectoryTree, RootRow } from '../directory-tree'
import { AreaSwitch } from './AreaSwitch'

// The folders, docked on the left. A narrow page drops the rail and leans on the breadcrumbs and folder tiles
export function FolderRail({
  tree,
  path,
  canManage,
  area,
  onArea,
  onNavigate,
  onNewFolder,
}: {
  tree: DirTreeNode[]
  path: string
  canManage: boolean
  area: MediaVisibility
  onArea?: (area: MediaVisibility) => void
  onNavigate: (path: string) => void
  onNewFolder: () => void
}) {
  return (
    <aside aria-label="Folders" className="hidden h-full w-60 shrink-0 flex-col border-r border-foreground/10 bg-background @min-[56rem]/media:flex">
      <div className="flex h-[49px] shrink-0 items-center justify-between border-b border-foreground/10 pr-2 pl-4">
        <h2 className="text-sm font-medium tracking-tight">Folders</h2>
        {canManage ? (
          <Button type="button" variant="ghost" size="icon" className="size-8 rounded-full text-muted-foreground" aria-label="New folder" title={path ? `New folder in ${path}` : 'New folder'} onClick={onNewFolder}>
            <FolderPlusIcon className="size-4" />
          </Button>
        ) : null}
      </div>
      {onArea ? <AreaSwitch area={area} onArea={onArea} className="mx-2 mt-2" /> : null}
      <div role="tree" aria-label="Folders" className="scrollbar-accent flex-1 overflow-y-auto p-2">
        <RootRow isActive={path === ''} tabbable={path === ''} onClick={() => onNavigate('')} label={area === 'PRIVATE' ? 'Private files' : 'Library'} />
        {tree.length > 0 ? <DirectoryTree nodes={tree} activePath={path} onNavigate={onNavigate} /> : null}
      </div>
    </aside>
  )
}
