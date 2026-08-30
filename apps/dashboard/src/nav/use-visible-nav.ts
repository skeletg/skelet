import { useMemo } from 'react'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'
import { isGroup, type NavLeaf, type NavNode, type NavSubgroup } from '@/nav/types'

// The one place the nav gating rules live: permission-gated items hide while grants are still
// loading (that flash read as a broken sidebar) and when the admin lacks the slug, feature-gated
// items hide when the server switch is off with a '!' prefix inverting the test, and a group whose
// every child got filtered out disappears with them
//
// `loading` matters to anything that acts once on the result - gated entries are simply absent
// until the grants and switches are in, so an early one-shot decision sees a short tree
export function useVisibleNav(items: NavNode[]): { nodes: NavNode[]; loading: boolean } {
  const { has, loading: permsLoading } = usePermissions()
  const { enabled: featureEnabled, loading: featuresLoading } = useFeatures()

  const nodes = useMemo(() => {
    const allowFeature = (feature?: string) => {
      if (!feature) return true
      if (featuresLoading) return false
      return feature.startsWith('!') ? !featureEnabled(feature.slice(1)) : featureEnabled(feature)
    }
    const allowLeaf = (leaf: NavLeaf) =>
      (!leaf.permission || (!permsLoading && has(leaf.permission))) && allowFeature(leaf.feature)

    const out: NavNode[] = []
    for (const item of items) {
      if (isGroup(item)) {
        if (item.subgroups) {
          const subgroups: NavSubgroup[] = item.subgroups
            .map((sg) => ({ ...sg, items: sg.items.filter(allowLeaf) }))
            .filter((sg) => sg.items.length > 0)
          if (subgroups.length > 0) out.push({ ...item, subgroups, children: undefined })
        } else {
          const children = (item.children ?? []).filter(allowLeaf)
          if (children.length > 0) out.push({ ...item, children })
        }
      } else if (allowLeaf(item)) {
        out.push(item)
      }
    }
    return out
  }, [items, permsLoading, has, featureEnabled, featuresLoading])

  return { nodes, loading: permsLoading || featuresLoading }
}
