import { useState, type ComponentProps } from 'react'
import { ChevronDownIcon } from 'lucide-react'
import { useConfigScope } from '@/lib/config-scope'
import { ConfigScopeFields } from '@/lib/config-scope-ui'
import { NavMain } from '@/components/layout/nav-main'
import { NavUser } from '@/components/layout/nav-user'
import { ChatTrigger } from '@/features/ai/ChatPanel'
import { NotificationBell } from '@/features/notifications/NotificationBell'
import { useIsMobile } from '@/hooks/use-mobile'
import { StoreClock } from '@/components/layout/StoreClock'
import { DevAdminSwitcher } from '@/shared/auth/DevAdminSwitcher'
import { getNavNodes } from '@/nav/registry'
import { cn } from '@/lib/utils'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { SidebarResizeRail } from '@/components/layout/sidebar-resize-rail'
import { sidebarThemeAttrs, useTheme } from '@/lib/theme'

export function AppSidebar(props: ComponentProps<typeof Sidebar>) {
  // The brand line doubles as a scope reminder - every list and save acts within this scope, so it should read at
  // a glance. Ellipsis only during the channels fetch, so a reload doesn't flash "Global scope" first

  // effectiveLocale rather than scope.locale: it's what the Locale header carries, so "Store default" shows the
  // resolved code - empty in Global scope, where no locale applies and the mark stays off
  const { selectedChannel, loadingChannels, effectiveLocale } = useConfigScope()
  const scopeLabel = selectedChannel?.name ?? (loadingChannels ? '…' : 'Global scope')
  const [scopeOpen, setScopeOpen] = useState(false)
  const isMobile = useIsMobile()
  // null whenever the rail follows the content, which leaves the markup exactly as it was
  const { appearance, custom, resolvedDark } = useTheme()
  const railTheme = sidebarThemeAttrs(appearance, custom, resolvedDark)
  return (
    <Sidebar
      collapsible="icon"
      variant={appearance.frame === 'docked' ? 'sidebar' : appearance.frame}
      side={appearance.sidebarSide}
      themeAttr={railTheme?.attr}
      themeDark={railTheme?.dark}
      themeStyle={railTheme?.style}
      {...props}
    >
      <SidebarHeader className="gap-0 p-0">
        <SidebarMenu>
          <SidebarMenuItem>
            {/* The brand block IS the scope switcher - clicking it unfolds the scope/channel/
                locale pickers INLINE below it, pushing the nav down, rather than floating a popup
                The row is the content header's height with only a bottom border, so the two
                dividers read as one line across the page */}
            <Collapsible open={scopeOpen} onOpenChange={setScopeOpen}>
              <div data-tour="scope">
              <div className="flex h-14 items-center border-b group-data-[variant=floating]:h-12 border-sidebar-border px-2">
              <CollapsibleTrigger asChild>
                <SidebarMenuButton
                  size="lg"
                  // the header reads as part of the bar, not a nav item - no fill on hover, press or while open
                  className="group-data-[collapsible=icon]:justify-center hover:bg-transparent hover:text-sidebar-foreground active:bg-transparent active:text-sidebar-foreground data-open:hover:bg-transparent data-open:hover:text-sidebar-foreground"
                >
                  {/* The favicon doubles as the brand mark - one file to swap when rebranding, and no bg tile since the logo brings its own colors */}
                  {/* Sized down to the nav icons in the collapsed rail: the button's [&_svg]:size-4
                      never reaches an img, so the mark stayed 24px above a column of 16px icons */}
                  <img
                    src="/favicon.svg"
                    alt=""
                    className="aspect-square size-6 shrink-0 rounded-lg group-data-[collapsible=icon]:size-4"
                  />

                  {/* Hidden rather than left to clip: the button keeps its width for them either
                      way, which is what pushed the mark off-centre in the rail */}
                  <div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="flex items-baseline gap-1.5">
                      <span className="truncate font-semibold">{scopeLabel}</span>
                      {effectiveLocale ? (
                        <span className="shrink-0 text-xs font-normal text-muted-foreground">
                          {effectiveLocale}
                        </span>
                      ) : null}
                    </span>
                    <span className="truncate text-xs">Admin panel</span>
                  </div>
                  <ChevronDownIcon
                    className={cn(
                      'ml-auto size-4 shrink-0 text-muted-foreground transition-transform',
                      'group-data-[collapsible=icon]:hidden',
                      scopeOpen && 'rotate-180',
                    )}
                  />
                </SidebarMenuButton>
              </CollapsibleTrigger>
              </div>
              <CollapsibleContent>
                {/* icon-collapsed sidebars have no room for the pickers - expand the rail first */}
                <div className="border-b border-sidebar-border px-3 pt-3 pb-3 group-data-[collapsible=icon]:hidden">
                  <ConfigScopeFields stacked idSuffix="-sb" />
                  <StoreClock />
                </div>
              </CollapsibleContent>
              </div>
            </Collapsible>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent data-tour="nav">
        <NavMain items={getNavNodes()} />
      </SidebarContent>
      <SidebarFooter>
        {/* The header has no room for these on a phone, so they live here instead - the header
            drops them at the same breakpoint, so exactly one instance of each is ever mounted */}
        {isMobile && (
          <div className="flex items-center gap-2 px-2 pb-1">
            <ChatTrigger />
            <NotificationBell />
          </div>
        )}
        {/* Dev-only identity switcher: tree-shaken from production
            bundles by the env guard; the backend additionally rejects
            the dev operations outside APP_ENV=development */}
        {import.meta.env.DEV && <DevAdminSwitcher />}
        <NavUser />
      </SidebarFooter>
      <SidebarResizeRail />
    </Sidebar>
  )
}
