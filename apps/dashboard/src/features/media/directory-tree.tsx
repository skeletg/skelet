import { useState } from 'react'
import { ChevronDownIcon, ChevronRightIcon, FolderIcon, FolderOpenIcon } from 'lucide-react'
import { onTreeItemKeyDown } from '@/lib/tree-keys'
import type { DirTreeNode } from './directory-paths'

// The media library's folder tree, shared by the library page and the media picker

export function DirectoryTree({
  nodes,
  activePath,
  onNavigate,
  depth = 0,
}: {
  nodes: DirTreeNode[]
  activePath: string
  onNavigate: (path: string) => void
  depth?: number
}) {
  return (
    <ul role={depth > 0 ? 'group' : 'none'} className="grid gap-0.5">
      {nodes.map((node) => (
        <DirectoryTreeNode key={node.path} node={node} activePath={activePath} onNavigate={onNavigate} depth={depth} />
      ))}
    </ul>
  )
}

function DirectoryTreeNode({
  node,
  activePath,
  onNavigate,
  depth,
}: {
  node: DirTreeNode
  activePath: string
  onNavigate: (path: string) => void
  depth: number
}) {
  const isActive = node.path === activePath
  const isAncestor = activePath.startsWith(node.path + '/')
  const [open, setOpen] = useState(isAncestor || isActive)
  const hasChildren = node.children.length > 0

  // Auto-expand when the node becomes the active path or an ancestor of it - State is adjusted during render instead of in an effect; both
  // flags are tracked separately so an active->ancestor transition still re-expands a manually collapsed node
  const [prevActivity, setPrevActivity] = useState({ isAncestor, isActive })
  if (prevActivity.isAncestor !== isAncestor || prevActivity.isActive !== isActive) {
    setPrevActivity({ isAncestor, isActive })
    if (isAncestor || isActive) setOpen(true)
  }

  return (
    <li role="none">
      <div
        role="treeitem"
        tabIndex={isActive ? 0 : -1}
        aria-level={depth + 2}
        aria-selected={isActive}
        aria-expanded={hasChildren ? open : undefined}
        onKeyDown={(e) => onTreeItemKeyDown(e, { expanded: open, hasChildren, onToggle: () => setOpen((v) => !v), onOpen: () => onNavigate(node.path) })}
        className={`group relative flex cursor-pointer items-center gap-1 rounded-md py-1.5 pr-2 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring ${
          isActive
            ? 'bg-primary/10 font-medium text-foreground'
            : isAncestor
              ? 'text-foreground hover:bg-muted/60'
              : 'text-foreground/90 hover:bg-muted/60'
        }`}
        style={{ paddingLeft: `${depth * 1.125 + 0.5}rem` }}
        onClick={() => onNavigate(node.path)}
      >
        {/* indent guides */}
        {depth > 0
          ? Array.from({ length: depth }).map((_, i) => (
              <span
                key={i}
                aria-hidden
                className="pointer-events-none absolute top-0 bottom-0 w-px bg-border/60"
                style={{ left: `${i * 1.125 + 0.875}rem` }}
              />
            ))
          : null}
        {/* The row's arrow keys expand and collapse, so this stays a mouse target out of the tab order */}
        <button
          type="button"
          tabIndex={-1}
          className="grid size-4 shrink-0 place-items-center text-muted-foreground hover:text-foreground"
          onClick={(e) => { e.stopPropagation(); setOpen((v) => !v) }}
          aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`}
          disabled={!hasChildren}
        >
          {hasChildren ? (
            open ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />
          ) : null}
        </button>
        {open && hasChildren ? (
          <FolderOpenIcon className={`size-4 shrink-0 ${isActive ? 'text-primary' : 'text-muted-foreground'}`} />
        ) : (
          <FolderIcon className={`size-4 shrink-0 ${isActive ? 'text-primary' : 'text-muted-foreground'}`} />
        )}
        <span className="flex-1 truncate">{node.name}</span>
      </div>
      {hasChildren && open ? (
        <DirectoryTree nodes={node.children} activePath={activePath} onNavigate={onNavigate} depth={depth + 1} />
      ) : null}
    </li>
  )
}

export function RootRow({ isActive, tabbable, onClick, label = 'Library' }: { isActive: boolean; tabbable: boolean; onClick: () => void; label?: string }) {
  return (
    <div
      role="treeitem"
      tabIndex={tabbable ? 0 : -1}
      aria-level={1}
      aria-selected={isActive}
      onClick={onClick}
      onKeyDown={(e) => onTreeItemKeyDown(e, { expanded: true, hasChildren: false, onToggle: () => {}, onOpen: onClick })}
      className={`mb-1 flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring ${
        isActive ? 'bg-primary/10 font-medium text-foreground' : 'text-foreground/90 hover:bg-muted/60'
      }`}
    >
      <FolderIcon className={`size-4 shrink-0 ${isActive ? 'text-primary' : 'text-muted-foreground'}`} />
      <span className="truncate">{label}</span>
    </div>
  )
}
