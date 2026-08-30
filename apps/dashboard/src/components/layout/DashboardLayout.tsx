import { useEffect, type CSSProperties, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { AdminPrefsProvider } from '@/lib/admin-prefs-provider'
import { ThemePrefSync } from '@/components/layout/theme-pref-sync'
import { LocalePrefSync } from '@/components/layout/locale-pref-sync'
import { LandingRedirect } from '@/components/layout/landing-redirect'
import { ScopeDefaultSync } from '@/components/layout/scope-default-sync'
import { useSidebarPref, useSidebarWidthPref } from '@/lib/sidebar-pref'
import { WelcomeProvider } from '@/features/onboarding/WelcomeDialog'
import { recordVisit } from '@/lib/recent-pages'
import { findActiveLeafPath } from '@/nav/active-path'
import { flattenLeaves } from '@/nav/types'
import { getNavNodes } from '@/nav/registry'
import { useAuth } from '@/shared/auth/auth'
import { useIsMobile } from '@/hooks/use-mobile'
import { AdminSearch } from '@/components/layout/admin-search'
import { AppSidebar } from '@/components/layout/app-sidebar'
import { TabPanels } from '@/components/layout/tab-panels'
import { ChatDock, ChatProvider, ChatTrigger } from '@/features/ai/ChatPanel'
import { PlaygroundModal, PlaygroundProvider } from '@/features/playground/PlaygroundModal'
import { NotificationBell } from '@/features/notifications/NotificationBell'
import { HeaderWorkspaceTabs } from '@/components/layout/workspace-tab-strip'
import { WorkspaceSwitcher } from '@/components/layout/workspace-switcher'
import { WorkspaceDndProvider } from '@/components/layout/workspace-dnd'
import { WorkspaceTabsProvider } from '@/lib/workspace-tabs'
import { scopePermissionForPath } from '@/nav/scope-permission'
import { ConfigScopeProvider } from '@/lib/config-scope-ui'
import { useUnloadGuard } from '@/lib/unsaved-changes'
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar'
import { Separator } from '@/components/ui/separator'

// Collapsed or expanded is a roaming preference, so the provider has to be controlled from inside
// AdminPrefsProvider - which is what makes it survive a reload at all, see sidebar-pref.ts
function ShellSidebarProvider({ children }: { children: ReactNode }) {
  const { open, setOpen } = useSidebarPref()
  // Width roams alongside collapsed/expanded - the rail drags it, Preferences has presets for it
  const { width } = useSidebarWidthPref()
  return (
    <SidebarProvider
      open={open}
      onOpenChange={setOpen}
      className="h-svh overflow-hidden"
      style={{ '--sidebar-width': `${width}px` } as CSSProperties}
    >
      {children}
    </SidebarProvider>
  )
}

// Without this the drawer stays open over the page you just opened - must sit inside SidebarProvider
function CloseMobileSidebarOnNavigate() {
  const { pathname } = useLocation()
  const { isMobile, setOpenMobile } = useSidebar()
  useEffect(() => {
    if (isMobile) setOpenMobile(false)
  }, [pathname, isMobile, setOpenMobile])
  return null
}

export function DashboardLayout() {
  const location = useLocation()
  const scopePermission = scopePermissionForPath(location.pathname)
  const { auth } = useAuth()
  const isMobile = useIsMobile()

  // One listener for the whole shell - it reads the dirty registry when the browser asks, so it
  // needs no state of its own and never re-subscribes
  useUnloadGuard()

  // recents resolve to the owning nav PAGE (longest prefix), so ten order details count as one
  // "Orders" entry rather than ten near-duplicates
  useEffect(() => {
    if (!auth) return
    const leaf = findActiveLeafPath(location.pathname, flattenLeaves(getNavNodes()).map((l) => l.path))
    if (leaf) recordVisit(auth.admin.id, leaf)
  }, [location.pathname, auth])

  return (
    // Wraps the WHOLE shell including the sidebar, since sidebar-mounted dev tooling reads the scope
    <ConfigScopeProvider permissionSlug={scopePermission}>
    <AdminPrefsProvider>
    {/* keyed by admin so the applied-signature ref resets when the signed-in admin changes */}
    <ThemePrefSync key={auth?.admin.id ?? ''} />
    <LocalePrefSync key={`locale-${auth?.admin.id ?? ''}`} />
    <LandingRedirect />
    <ScopeDefaultSync />
    <WorkspaceTabsProvider>
    {/* one drag context over the header strip AND the content area - a tab has to travel between them */}
    <WorkspaceDndProvider>
    <WelcomeProvider>
    <ShellSidebarProvider>
      {/* Above the sidebar, not just the inset: on a phone the AI trigger lives in the sidebar
          footer, and it needs this context to open the panel */}
      <ChatProvider>
      <AppSidebar />
      <CloseMobileSidebarOnNavigate />
      <SidebarInset className="min-h-0 overflow-hidden">
        <PlaygroundProvider>
          {/* px-2 matches the sidebar's own gutter, so the toggle hugs the edge with the same rhythm */}
          {/* Deliberately NOT flex-wrap: wrapping is decided on natural content widths BEFORE
              shrink applies, so a long title or channel name broke onto a second row even though
              every element here truncates - nowrap makes long text truncate instead, and each
              breakpoint below is sized so the rigid parts always fit one row */}
          <header className="flex min-h-14 shrink-0 items-center gap-2 border-b px-2 py-2">
            <SidebarTrigger className="size-8" />
            {/* -my-2 cancels the header's py-2 so the divider spans the full height */}
            <Separator orientation="vertical" className="-my-2" />
            {/* Which saved arrangement is on screen - it owns the tabs beside it, so it reads left
                of them, and it keeps the layout actions that only apply once a workspace is split */}
            <WorkspaceSwitcher />
            <Separator orientation="vertical" className="-my-2" />
            {/* Chrome-style workspace tabs replace the old back button + page name - navigation
                stays in the active tab and New Tab is the only way another one appears */}
            <HeaderWorkspaceTabs />
            {/* min-w-0 and no shrink-0: under extreme narrowness the scope button truncates alongside the tabs rather than pushing the row into overflow */}
            {/* scope/channel/locale pickers moved into the sidebar's brand block (the scope
                switcher) - the global admin search palette took their spot */}
            {/* On a phone the AI trigger and the bell move into the sidebar footer, so the header
                keeps room for the tab strip. Rendered in ONE place, never hidden with CSS in the
                other - NotificationBell holds a GraphQL subscription and a second mount would
                open a second one */}
            <div className="ml-auto flex min-w-0 items-center gap-2">
              <AdminSearch />
              {!isMobile && (
                <>
                  <ChatTrigger />
                  <NotificationBell />
                </>
              )}
            </div>
          </header>
          {/* main is a non-scrolling flex column and the page wrapper takes the rest via flex-1,
              giving the DEFINITE height percentage heights and sticky headers need to resolve */}
          {/* The AI panel is a real layout column, so opening it shrinks main instead of overlaying */}
          <div className="flex min-h-0 flex-1 overflow-hidden">
            <main className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
              {/* every workspace tab keeps its page mounted in here - switching tabs shows and
                  hides panels instead of remounting, so page state and scroll survive */}
              <TabPanels />
            </main>
            <ChatDock />
          </div>
          <PlaygroundModal />
        </PlaygroundProvider>
      </SidebarInset>
      </ChatProvider>
    </ShellSidebarProvider>
    </WelcomeProvider>
    </WorkspaceDndProvider>
    </WorkspaceTabsProvider>
    </AdminPrefsProvider>
    </ConfigScopeProvider>
  )
}
