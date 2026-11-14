export type FlatCategory = { id: string; label: string; urlPath: string }

// Structural shape of the category tree nodes this flattener walks; the generated query types inline each nesting
// level, so a recursive hand-written type is the practical way to accept the whole tree
type CategoryTreeNode = {
  id: string
  slug: string
  urlPath: string
  content?: { name: string } | null
  children?: CategoryTreeNode[] | null
}

export function flattenCategories(roots: CategoryTreeNode[], depth = 0, out: FlatCategory[] = []): FlatCategory[] {
  for (const c of roots) {
    const name = c.content?.name ?? c.slug
    out.push({
      id: c.id,
      label: `${'– '.repeat(depth)}${name}`,
      urlPath: c.urlPath,
    })
    if (c.children?.length) flattenCategories(c.children, depth + 1, out)
  }
  return out
}
