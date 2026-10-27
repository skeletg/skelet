// Folder paths into the tree the library page and the media picker draw, and into breadcrumbs

export type DirTreeNode = {
  name: string
  path: string
  children: DirTreeNode[]
}

export function buildDirectoryTree(paths: string[]): DirTreeNode[] {
  const root: DirTreeNode[] = []
  for (const dirPath of [...paths].sort()) {
    const parts = dirPath.split('/').filter(Boolean)
    let nodes = root
    for (let i = 0; i < parts.length; i++) {
      const fullPath = parts.slice(0, i + 1).join('/')
      let node = nodes.find((n) => n.name === parts[i])
      if (!node) {
        node = { name: parts[i], path: fullPath, children: [] }
        nodes.push(node)
      }
      nodes = node.children
    }
  }
  return root
}

export function buildBreadcrumbs(path: string): { name: string; path: string }[] {
  const crumbs = [{ name: 'Library', path: '' }]
  if (!path) return crumbs
  const parts = path.split('/').filter(Boolean)
  for (let i = 0; i < parts.length; i += 1) {
    crumbs.push({ name: parts[i], path: parts.slice(0, i + 1).join('/') })
  }
  return crumbs
}
