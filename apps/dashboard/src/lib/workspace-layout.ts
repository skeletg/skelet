import { randomId } from '@/lib/random-id'

// The split-view tree behind the workspace tabs: leaves are panes (each with its own strip and its
// own mounted pages) and splits stack them in a row or column
//
// sizes are flex-grow WEIGHTS rather than pixels or percentages - dividers eat real width, and a
// weight stays meaningful when the window resizes or a sibling disappears

export type Edge = 'left' | 'right' | 'top' | 'bottom'
export type PaneNode = { kind: 'pane'; id: string }
export type SplitNode = {
  kind: 'split'
  id: string
  dir: 'row' | 'col'
  sizes: number[]
  children: LayoutNode[]
}
export type LayoutNode = PaneNode | SplitNode

export function paneNode(id: string): PaneNode {
  return { kind: 'pane', id }
}

export function splitDir(edge: Edge): 'row' | 'col' {
  return edge === 'left' || edge === 'right' ? 'row' : 'col'
}

function insertsBefore(edge: Edge): boolean {
  return edge === 'left' || edge === 'top'
}

export function layoutPaneIds(node: LayoutNode): string[] {
  return node.kind === 'pane' ? [node.id] : node.children.flatMap(layoutPaneIds)
}

/** Drops a new pane against one edge of an existing one - the target gives up half its weight */
export function splitPane(
  root: LayoutNode,
  targetId: string,
  newPaneId: string,
  edge: Edge,
): LayoutNode {
  const dir = splitDir(edge)
  if (root.kind === 'split') {
    const i = root.children.findIndex((c) => c.kind === 'pane' && c.id === targetId)
    // a split already running this way absorbs the newcomer as a sibling - nesting another level
    // would give the same picture with an extra divider that resizes the wrong things
    if (i >= 0 && root.dir === dir) {
      const at = insertsBefore(edge) ? i : i + 1
      const half = root.sizes[i] / 2
      const sizes = [...root.sizes]
      sizes[i] = half
      sizes.splice(at, 0, half)
      const children = [...root.children]
      children.splice(at, 0, paneNode(newPaneId))
      return { ...root, sizes, children }
    }
    return { ...root, children: root.children.map((c) => splitPane(c, targetId, newPaneId, edge)) }
  }
  if (root.id !== targetId) return root
  const pair = insertsBefore(edge)
    ? [paneNode(newPaneId), root]
    : [root, paneNode(newPaneId)]
  return { kind: 'split', id: randomId(), dir, sizes: [50, 50], children: pair }
}

/** Null means the tree emptied out - the caller decides what an empty workspace turns into */
export function removePane(root: LayoutNode, paneId: string): LayoutNode | null {
  if (root.kind === 'pane') return root.id === paneId ? null : root
  const children: LayoutNode[] = []
  const sizes: number[] = []
  root.children.forEach((child, i) => {
    const next = removePane(child, paneId)
    if (!next) return
    children.push(next)
    sizes.push(root.sizes[i])
  })
  if (children.length === 0) return null
  // a split down to one child IS that child - leaving the wrapper would strand a divider-less split
  if (children.length === 1) return children[0]
  const total = sizes.reduce((a, b) => a + b, 0) || 1
  return { ...root, children, sizes: sizes.map((s) => (s / total) * 100) }
}

export function setSplitSizes(root: LayoutNode, splitId: string, sizes: number[]): LayoutNode {
  if (root.kind === 'pane') return root
  if (root.id === splitId) return { ...root, sizes }
  return { ...root, children: root.children.map((c) => setSplitSizes(c, splitId, sizes)) }
}

// Storage is user-editable and survives across builds, so a restored tree is guilty until proven
// well formed - anything off and the caller falls back to a flat layout it builds itself
export function parseLayout(raw: unknown): LayoutNode | null {
  if (!raw || typeof raw !== 'object') return null
  const node = raw as Partial<SplitNode> & Partial<PaneNode>
  if (node.kind === 'pane') return typeof node.id === 'string' ? { kind: 'pane', id: node.id } : null
  if (node.kind !== 'split') return null
  if (!Array.isArray(node.children) || !Array.isArray(node.sizes)) return null
  if (node.children.length !== node.sizes.length || node.children.length < 2) return null
  if (node.dir !== 'row' && node.dir !== 'col') return null
  const children = node.children.map(parseLayout)
  if (children.some((c) => c === null)) return null
  const sizes = node.sizes.map((s) => (typeof s === 'number' && s > 0 ? s : 1))
  return {
    kind: 'split',
    id: typeof node.id === 'string' ? node.id : randomId(),
    dir: node.dir,
    sizes,
    children: children as LayoutNode[],
  }
}
