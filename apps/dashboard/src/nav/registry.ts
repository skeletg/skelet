import {
  flattenLeaves,
  isGroup,
  type NavGroup,
  type NavLeaf,
  type NavNode,
  type NavSubgroup,
} from './types'

const navNodes: NavNode[] = []
const footerNodes: NavNode[] = []
const detailRoutes: NavLeaf[] = []

function assertUniquePath(path: string) {
  const all = [
    ...flattenLeaves(navNodes).map((l) => l.path),
    ...flattenLeaves(footerNodes).map((l) => l.path),
    ...detailRoutes.map((l) => l.path),
  ]
  if (all.includes(path)) {
    throw new Error(`nav registry: duplicate path "${path}"`)
  }
}

function leafPathsIn(node: NavNode): string[] {
  return isGroup(node) ? flattenLeaves([node]).map((l) => l.path) : [node.path]
}

/** A leaf, or a group with children or subgroups - throws on duplicate paths, and on a group with
 *  no `id`, since that is what `extendGroup` targets and what a hidden-group preference stores */
export function registerNavNode(node: NavNode) {
  for (const path of leafPathsIn(node)) assertUniquePath(path)
  if (isGroup(node) && !node.id) {
    throw new Error(`nav registry: group "${node.title}" needs an id`)
  }
  navNodes.push(node)
}

/** Renders in the sidebar footer - empty in core, reserved for channel-style pinning */
export function registerFooterNode(node: NavNode) {
  for (const path of leafPathsIn(node)) assertUniquePath(path)
  footerNodes.push(node)
}

/** Router-only route with no sidebar entry - for dynamic detail pages like `/orders/:id` */
export function registerDetailRoute(route: NavLeaf) {
  assertUniquePath(route.path)
  detailRoutes.push(route)
}

/** Append into an existing group by `id`, for plugins adding to a core group rather than a new
 *  top-level one - with subgroups, items land in the last unless `subgroupLabel` names one
 *  (created if missing) */
export function extendGroup(
  id: string,
  items: NavLeaf[],
  opts?: { subgroupLabel?: string },
) {
  const group = navNodes.find((n): n is NavGroup => isGroup(n) && n.id === id)
    ?? footerNodes.find((n): n is NavGroup => isGroup(n) && n.id === id)
  if (!group) {
    throw new Error(`nav registry: extendGroup target "${id}" not found`)
  }
  for (const leaf of items) assertUniquePath(leaf.path)

  if (group.subgroups) {
    const label = opts?.subgroupLabel
    let target: NavSubgroup | undefined
    if (label != null) {
      target = group.subgroups.find((sg) => sg.label === label)
      if (!target) {
        target = { label, items: [] }
        group.subgroups.push(target)
      }
    } else {
      target = group.subgroups[group.subgroups.length - 1]
    }
    target.items.push(...items)
    return
  }
  group.children = [...(group.children ?? []), ...items]
}

export function getNavNodes(): NavNode[] {
  return navNodes
}

export function getFooterNodes(): NavNode[] {
  return footerNodes
}

export function getDetailRoutes(): NavLeaf[] {
  return detailRoutes
}

/** Flattened nav + footer + detail-only routes - App.tsx maps this into <Route> elements */
export function allRouterRoutes(): NavLeaf[] {
  return [...flattenLeaves(navNodes), ...flattenLeaves(footerNodes), ...detailRoutes].filter(
    (l) => l.element,
  )
}

export function __resetRegistryForTests() {
  navNodes.length = 0
  footerNodes.length = 0
  detailRoutes.length = 0
}
