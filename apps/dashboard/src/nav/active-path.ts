import { useContext } from 'react'
import { UNSAFE_LocationContext } from 'react-router-dom'
import { getDetailRoutes, getFooterNodes, getNavNodes } from '@/nav/registry'
import { flattenLeaves } from '@/nav/types'

// Which registered page a url belongs to, and what sits above a subpage
//
// Deliberately its own module rather than living in nav-main: every detail page reads this, and
// importing it from the sidebar would drag the whole nav tree into all ~100 of those chunks

/** The longest registered nav path that owns `currentPath` - `/cms/pages/abc` resolves to `/cms/pages` */
export function findActiveLeafPath(currentPath: string, paths: string[]): string | null {
  let best: string | null = null
  for (const p of paths) {
    const matches =
      p === '/'
        ? currentPath === '/'
        : currentPath === p || currentPath.startsWith(p + '/')
    if (matches && (!best || p.length > best.length)) best = p
  }
  return best
}

export type ParentPage = { path: string; title: string }

/**
 * The page a subpage should offer a way back to, or null when this URL *is* a registered page.
 *
 * Deriving it beats a prop on every detail page: a subpage can't forget it, and a route that moves
 * in the nav registry takes its own back link with it. Reads the panel's own location, which the
 * workspace tabs freeze per tab, so a background tab never resolves against the visible url
 */
/**
 * The nearest registered page strictly ABOVE this url.
 *
 * Strict is the whole rule: a page is never its own parent, so a destination like /preferences gets
 * nothing, while /cms/pages/<id> and /cms/page-types/new both find the list they came from - the
 * second only works because "is it deeper?" is asked of the url, not of whether the page happens to
 * be registered in its own right.
 *
 * Patterns are skipped because `/admins/:id` is not somewhere a link can go.
 */
export function parentFromLeaves(
  currentPath: string,
  leaves: { path: string; title: string }[],
): ParentPage | null {
  let best: { path: string; title: string } | null = null
  for (const leaf of leaves) {
    if (leaf.path.includes(':') || leaf.path === '/') continue
    if (!currentPath.startsWith(leaf.path + '/')) continue
    if (!best || leaf.path.length > best.path.length) best = leaf
  }
  return best ? { path: best.path, title: best.title } : null
}

// Every registered page, not just the sidebar: the list a detail page came from is often route-only
// (reached from a hub card), and reading only the sidebar sent /cms/pages/<id> back to the CMS hub
export function allRegisteredPages(): { path: string; title: string }[] {
  return [...flattenLeaves(getNavNodes()), ...flattenLeaves(getFooterNodes()), ...getDetailRoutes()]
}

export function findParentPage(currentPath: string): ParentPage | null {
  return parentFromLeaves(currentPath, allRegisteredPages())
}

// Reads the location through the context rather than useLocation() so the header stays renderable
// outside a Router - it was before this hook existed, and a unit test that mounts a page in
// isolation shouldn't have to grow a MemoryRouter to keep working
export function useParentPage(): ParentPage | null {
  const ctx = useContext(UNSAFE_LocationContext)
  const pathname = ctx?.location?.pathname
  return pathname ? findParentPage(pathname) : null
}
