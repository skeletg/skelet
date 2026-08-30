import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { APP_NAME, PageTitleContext, resolveRouteTitle, type Override } from './page-title'

// Split from page-title so that module exports only hooks - fast refresh needs the separation

export function PageTitleProvider({ children }: { children: ReactNode }) {
  const { pathname } = useLocation()
  const [override, setOverride] = useState<Override | null>(null)

  const setPageTitle = useCallback((path: string, title: string | null) => {
    setOverride((prev) => {
      if (title) return { pathname: path, title }
      // Only discard the override if this path set it, so a fast route change can't wipe the incoming page's
      return prev && prev.pathname === path ? null : prev
    })
  }, [])

  const routeTitle = useMemo(() => resolveRouteTitle(pathname), [pathname])
  // A stale override from the page we just left would otherwise leak onto the new one
  const isOverride = !!override && override.pathname === pathname
  const effective = isOverride ? override.title : routeTitle

  useEffect(() => {
    document.title = effective ? `${effective} · ${APP_NAME}` : APP_NAME
  }, [effective])

  const value = useMemo(
    () => ({ setPageTitle, title: effective, isOverride }),
    [setPageTitle, effective, isOverride],
  )
  return (
    <PageTitleContext.Provider value={value}>
      {children}
    </PageTitleContext.Provider>
  )
}
