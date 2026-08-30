import type { ReactNode } from 'react'

export type NavLeaf = {
  title: string
  path: string
  icon?: ReactNode
  subtitle?: string // muted second line under the title
  element?: ReactNode // omitted for action leaves - they open UI in place and register no route
  actionId?: string // renders a button that runs the matching nav action instead of a link
  permission?: string // hidden unless the admin holds it, or is superuser
  feature?: string // server feature switch, e.g. 'b2b' - a '!' prefix inverts it, so '!signals' shows only when OFF
}

export type NavSubgroup = {
  label?: string // small heading above this subgroup's items
  items: NavLeaf[]
}

export type NavGroup = {
  id?: string // required by registerNavNode - extendGroup targets it, and the hidden-groups preference stores it
  title: string
  icon?: ReactNode
  defaultOpen?: boolean
  flyout?: boolean // always use a flyout popover instead of expanding inline
  children?: NavLeaf[] // mutually exclusive with `subgroups`
  subgroups?: NavSubgroup[] // visually divided sub-sections - takes precedence over `children` when rendering
}

export type NavNode = NavLeaf | NavGroup

export function isGroup(node: NavNode): node is NavGroup {
  return 'children' in node || 'subgroups' in node
}

export function groupItems(group: NavGroup): NavLeaf[] { // flattens whether the group uses `children` or `subgroups`
  if (group.subgroups) return group.subgroups.flatMap((s) => s.items)
  return group.children ?? []
}

export function flattenLeaves(nodes: NavNode[]): NavLeaf[] {
  return nodes.flatMap((n) => (isGroup(n) ? groupItems(n) : [n]))
}
