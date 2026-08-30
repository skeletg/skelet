import { useMemo, useState, type ComponentPropsWithoutRef, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { runNavAction } from '@/nav/actions'
import { ChevronRightIcon, StarIcon } from 'lucide-react'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { Trans } from '@lingui/react/macro'
import { cn } from '@/lib/utils'
import { findActiveLeafPath } from '@/nav/active-path'
import { usePinnedPaths } from '@/lib/nav-pins'
import { useHiddenGroups } from '@/lib/nav-hidden'
import { useVisibleNav } from '@/nav/use-visible-nav'
import { useNavLabel } from '@/nav/nav-i18n'
import {
  flattenLeaves,
  groupItems,
  isGroup,
  type NavGroup,
  type NavLeaf,
  type NavNode,
} from '@/nav/types'

// A touch more breathing room than shadcn's defaults (h-8/text-sm for top
// level, h-7 for sub) - Applied via className so the primitive keeps all its
// behavior (tooltip sizing, icon-rail collapse) - The active state uses a
// tinted sidebar-primary bg + colored text so it reads clearly without the
// heavy solid fill a full `bg-primary` gives in the default theme
const ACTIVE_STATE =
  'data-active:bg-sidebar-primary/10 data-active:text-sidebar-primary data-active:font-semibold data-active:hover:bg-sidebar-primary/15 data-active:hover:text-sidebar-primary'
const MENU_BUTTON = `h-10 gap-2.5 text-[15px] ${ACTIVE_STATE}`
const MENU_SUB_BUTTON = `h-8 text-sm ${ACTIVE_STATE}`

// Action leaves carry no route, so they render as a button - both branches must forward what Radix's asChild Slot injects, or the item loses its sidebar styling entirely
function LeafTarget({
  leaf,
  children,
  ...slotted
}: { leaf: NavLeaf; children: ReactNode } & ComponentPropsWithoutRef<'a'>) {
  if (leaf.actionId) {
    const id = leaf.actionId
    return (
      <button
        {...(slotted as ComponentPropsWithoutRef<'button'>)}
        type="button"
        onClick={() => runNavAction(id)}
      >
        {children}
      </button>
    )
  }
  return (
    <NavLink {...slotted} to={leaf.path} end>
      {children}
    </NavLink>
  )
}

// Hover star on every nav row - pinning hoists the page into the per-admin Pinned group above the main nav; sits as a
// SIBLING of the row's link (absolute, like SidebarMenuAction) since a button nested inside an anchor is invalid markup
function PinStar({ path }: { path: string }) {
  const { isPinned, toggle } = usePinnedPaths()
  const pinned = isPinned(path)
  return (
    <button
      type="button"
      title={pinned ? 'Unpin from sidebar' : 'Pin to sidebar'}
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        toggle(path)
      }}
      className={cn(
        'absolute top-1/2 right-1.5 -translate-y-1/2 rounded-md p-1 hover:bg-sidebar-accent',
        pinned
          ? 'text-sidebar-primary'
          : 'text-sidebar-foreground/50 opacity-0 group-hover/navrow:opacity-100 focus-visible:opacity-100',
      )}
    >
      <StarIcon className={cn('size-3.5', pinned && 'fill-current')} />
    </button>
  )
}

export function NavMain({ items }: { items: NavNode[] }) {
  const { state, isMobile } = useSidebar()
  const isIconOnly = !isMobile && state === 'collapsed'

  const navLabel = useNavLabel()
  const { nodes: visible } = useVisibleNav(items)
  // Hiding a group only tidies the sidebar - pins below still resolve against the full tree, so
  // you can hide a group and keep the one page from it you use
  const { isHidden } = useHiddenGroups()
  const shown = visible.filter((n) => !(isGroup(n) && n.id && isHidden(n.id)))

  const location = useLocation()
  const activePath = useMemo(() => {
    const paths = flattenLeaves(visible).map((l) => l.path)
    return findActiveLeafPath(location.pathname, paths)
  }, [visible, location.pathname])

  // Pins resolve against the VISIBLE leaves, so titles and permission gates stay live and a
  // stale pin (renamed route, revoked permission) silently drops instead of dead-linking
  const { pins } = usePinnedPaths()
  const pinnedLeaves = useMemo(() => {
    const byPath = new Map(flattenLeaves(visible).map((l) => [l.path, l]))
    return pins.map((p) => byPath.get(p)).filter((l): l is NavLeaf => !!l)
  }, [pins, visible])

  return (
    <>
    {pinnedLeaves.length > 0 && (
      <SidebarGroup className="pb-0">
        <SidebarGroupLabel><Trans>Pinned</Trans></SidebarGroupLabel>
        <SidebarMenu className="gap-0.5">
          {pinnedLeaves.map((leaf) => (
            <SidebarMenuItem key={leaf.path} className="group/navrow relative">
              <SidebarMenuButton
                asChild
                tooltip={navLabel(leaf.title)}
                isActive={leaf.path === activePath}
                className={MENU_BUTTON}
              >
                <LeafTarget leaf={leaf}>
                  {leaf.icon}
                  <span>{navLabel(leaf.title)}</span>
                </LeafTarget>
              </SidebarMenuButton>
              {!isIconOnly && <PinStar path={leaf.path} />}
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroup>
    )}
    <SidebarGroup>
      <SidebarMenu className="gap-0.5">
        {shown.map((item) =>
          isGroup(item) ? (
            isIconOnly || item.flyout ? (
              <CollapsedGroup key={item.title} item={item} activePath={activePath} />
            ) : (
              <ExpandedGroup key={item.title} item={item} activePath={activePath} />
            )
          ) : (
            <SidebarMenuItem key={item.path} className="group/navrow relative">
              <SidebarMenuButton
                asChild
                tooltip={navLabel(item.title)}
                isActive={item.path === activePath}
                className={MENU_BUTTON}
              >
                <LeafTarget leaf={item}>
                  {item.icon}
                  <span>{navLabel(item.title)}</span>
                </LeafTarget>
              </SidebarMenuButton>
              {!isIconOnly && <PinStar path={item.path} />}
            </SidebarMenuItem>
          ),
        )}
      </SidebarMenu>
    </SidebarGroup>
    </>
  )
}

// Inline collapsible - auto-opens when one of its children is active, but
// stays user-controlled after that so manual collapse/expand still works
function ExpandedGroup({ item, activePath }: { item: NavGroup; activePath: string | null }) {
  const navLabel = useNavLabel()
  const allItems = groupItems(item)
  const hasActiveChild = allItems.some((c) => c.path === activePath)
  const [open, setOpen] = useState(hasActiveChild || !!item.defaultOpen)

  // Auto-open when a child route becomes active; the group stays user-collapsible afterwards (state adjustment during
  // render, keyed on the hasActiveChild transition)
  const [prevHasActiveChild, setPrevHasActiveChild] = useState(hasActiveChild)
  if (prevHasActiveChild !== hasActiveChild) {
    setPrevHasActiveChild(hasActiveChild)
    if (hasActiveChild) setOpen(true)
  }

  function renderLeaf(sub: NavLeaf) {
    return (
      <SidebarMenuSubItem key={sub.path} className="group/navrow relative">
        <SidebarMenuSubButton
          asChild
          isActive={sub.path === activePath}
          className={sub.subtitle ? `h-auto py-1.5 ${ACTIVE_STATE}` : MENU_SUB_BUTTON}
        >
          <LeafTarget leaf={sub}>
            {sub.subtitle ? (
              <span className="flex min-w-0 flex-col gap-0.5 leading-tight">
                <span className="text-sm">{navLabel(sub.title)}</span>
                <span className="truncate text-xs font-normal text-sidebar-foreground/55">{navLabel(sub.subtitle)}</span>
              </span>
            ) : (
              <span>{navLabel(sub.title)}</span>
            )}
          </LeafTarget>
        </SidebarMenuSubButton>
        <PinStar path={sub.path} />
      </SidebarMenuSubItem>
    )
  }

  return (
    <Collapsible asChild open={open} onOpenChange={setOpen} className="group/collapsible">
      <SidebarMenuItem>
        <CollapsibleTrigger asChild>
          {/* Parent doesn't highlight when a child is active - the open state
              + the child's own highlight carry that information */}
          <SidebarMenuButton tooltip={navLabel(item.title)} className={MENU_BUTTON}>
            {item.icon}
            <span>{navLabel(item.title)}</span>
            <ChevronRightIcon className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
          </SidebarMenuButton>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarMenuSub className="gap-0.5">
            {item.subgroups
              ? item.subgroups.map((sg, i) => (
                  <div key={sg.label ?? `__sub-${i}`} className="grid gap-0.5">
                    {sg.label ? (
                      <div className="mt-1.5 px-2 pt-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-sidebar-foreground/50">
                        {navLabel(sg.label)}
                      </div>
                    ) : null}
                    {sg.items.map(renderLeaf)}
                  </div>
                ))
              : (item.children ?? []).map(renderLeaf)}
          </SidebarMenuSub>
        </CollapsibleContent>
      </SidebarMenuItem>
    </Collapsible>
  )
}

// Flyout popover - the icon button highlights if any child is active;
// inside the popover the active child is visually marked too
function CollapsedGroup({ item, activePath }: { item: NavGroup; activePath: string | null }) {
  const navLabel = useNavLabel()
  const allItems = groupItems(item)
  const hasActiveChild = allItems.some((c) => c.path === activePath)

  function renderLeaf(sub: NavLeaf) {
    return (
      <DropdownMenuItem
        key={sub.path}
        asChild
        className={cn(
          sub.path === activePath &&
            'bg-primary/10 font-semibold text-primary focus:bg-primary/15 focus:text-primary',
        )}
      >
        <LeafTarget leaf={sub}>
          {sub.subtitle ? (
            <span className="flex flex-col gap-0.5 leading-tight">
              <span>{navLabel(sub.title)}</span>
              <span className="text-xs font-normal text-muted-foreground">{navLabel(sub.subtitle)}</span>
            </span>
          ) : (
            navLabel(sub.title)
          )}
        </LeafTarget>
      </DropdownMenuItem>
    )
  }

  return (
    <SidebarMenuItem>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <SidebarMenuButton tooltip={navLabel(item.title)} isActive={hasActiveChild} className={MENU_BUTTON}>
            {item.icon}
            <span>{navLabel(item.title)}</span>
          </SidebarMenuButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="right" align="start" sideOffset={8} className="min-w-44">
          <DropdownMenuLabel>{navLabel(item.title)}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {item.subgroups
            ? item.subgroups.map((sg, i) => (
                <div key={sg.label ?? `__sub-${i}`}>
                  {i > 0 || sg.label ? <DropdownMenuSeparator /> : null}
                  {sg.label ? (
                    <DropdownMenuLabel className="text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
                      {navLabel(sg.label)}
                    </DropdownMenuLabel>
                  ) : null}
                  {sg.items.map(renderLeaf)}
                </div>
              ))
            : (item.children ?? []).map(renderLeaf)}
        </DropdownMenuContent>
      </DropdownMenu>
    </SidebarMenuItem>
  )
}
