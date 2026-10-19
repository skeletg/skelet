import { isValidElement, useMemo } from 'react'
import { Navigate } from 'react-router-dom'
import { getDetailRoutes, getNavNodes } from '@/nav/registry'
import { groupItems, isGroup, type NavLeaf } from '@/nav/types'
import { useVisibleNav } from '@/nav/use-visible-nav'

// Every page this admin can actually reach, flattened - the ⌘K palette lists them and the landing preference validates against them.
// Deliberately blind to hidden sidebar groups: hiding a group tidies the sidebar, it doesn't take the pages away
export type VisiblePage = { path: string; title: string; context?: string }

// Route-only pages count too, or ⌘K missed a third of the dashboard - all but parameterised paths and redirects
const isStaticPage = (l: NavLeaf) =>
  !l.path.includes(':') && !!l.element && !(isValidElement(l.element) && l.element.type === Navigate)

export function useVisiblePages(): { pages: VisiblePage[]; loading: boolean } {
  const { nodes, loading } = useVisibleNav(getNavNodes())
  const detailRoutes = useMemo(() => getDetailRoutes().filter(isStaticPage), [])
  const { nodes: details, loading: detailsLoading } = useVisibleNav(detailRoutes)

  const pages = useMemo(() => {
    const out: VisiblePage[] = []
    for (const node of nodes) {
      if (isGroup(node)) {
        for (const leaf of groupItems(node)) {
          out.push({ path: leaf.path, title: leaf.title, context: node.title })
        }
      } else {
        out.push({ path: node.path, title: node.title })
      }
    }
    // A route-only page borrows its context from the nav page it sits under, /cms/tags from /cms's group
    const navPages = [...out]
    for (const leaf of details as NavLeaf[]) {
      const parent = navPages
        .filter((p) => p.path !== '/' && leaf.path.startsWith(`${p.path}/`))
        .sort((a, b) => b.path.length - a.path.length)[0]
      out.push({ path: leaf.path, title: leaf.title, context: parent ? (parent.context ?? parent.title) : undefined })
    }
    return out
  }, [nodes, details])

  return { pages, loading: loading || detailsLoading }
}
