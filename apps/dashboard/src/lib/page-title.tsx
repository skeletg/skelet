import { createContext, useContext, useEffect } from 'react'
import { matchRoutes, useLocation } from 'react-router-dom'
import { allRouterRoutes } from '@/nav/registry'
import { useTabPanelActive } from '@/lib/tab-panel-context'

// Drives document.title from each page's registered nav route rather than one static app name,
// overridable via usePageTitle() - mount the provider once, inside the router

export const APP_NAME = 'Skelet'

// Honours dynamic segments via React Router's own ranking, so /orders/new beats /orders/:id
export function resolveRouteTitle(pathname: string): string | null {
  const routes = allRouterRoutes()
  const matches = matchRoutes(
    routes.map((leaf) => ({ path: leaf.path })),
    pathname,
  )
  const matchedPath = matches?.[matches.length - 1]?.route.path
  if (!matchedPath) return null
  return routes.find((leaf) => leaf.path === matchedPath)?.title ?? null
}

export type Override = { pathname: string; title: string }

export const PageTitleContext = createContext<{
  setPageTitle: (pathname: string, title: string | null) => void
  title: string | null
  isOverride: boolean
}>({ setPageTitle: () => {}, title: null, isOverride: false })

// The resolved title of the page currently on screen - isOverride distinguishes a page-supplied
// record title ("Order #123") from the route default ("Order detail"), which the workspace tab
// strip needs to keep a revisited tab's richer label from flashing back to the generic one
export function useResolvedPageTitle(): { title: string | null; isOverride: boolean } {
  const { title, isOverride } = useContext(PageTitleContext)
  return { title, isOverride }
}

// Overrides the route default with a record-specific title - null while loading keeps the default
// Pages in HIDDEN workspace tabs stay mounted, so only the visible panel may hold the override -
// without the gate a background detail page would keep stomping the active page's title
export function usePageTitle(title: string | null | undefined) {
  const active = useTabPanelActive()
  const { pathname } = useLocation()
  const { setPageTitle } = useContext(PageTitleContext)
  useEffect(() => {
    if (!active) return
    setPageTitle(pathname, title ?? null)
    return () => setPageTitle(pathname, null)
  }, [active, pathname, title, setPageTitle])
}
