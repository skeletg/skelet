import { useMemo } from 'react'
import { getNavNodes } from '@/nav/registry'
import { groupItems, isGroup } from '@/nav/types'
import { useVisibleNav } from '@/nav/use-visible-nav'

// Every nav page this admin can actually reach, flattened - the ⌘K palette lists them and the landing preference validates against them.
// Deliberately blind to hidden sidebar groups: hiding a group tidies the sidebar, it doesn't take the pages away
export type VisiblePage = { path: string; title: string; context?: string }

export function useVisiblePages(): { pages: VisiblePage[]; loading: boolean } {
  const { nodes, loading } = useVisibleNav(getNavNodes())

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
    // route-only pages aren't in the nav registry, but this one should still be findable
    out.push({ path: '/preferences', title: 'Preferences', context: 'Account' })
    return out
  }, [nodes])

  return { pages, loading }
}
