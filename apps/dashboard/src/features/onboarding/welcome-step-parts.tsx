import type { ReactNode } from 'react'
import { getNavNodes } from '@/nav/registry'
import { isGroup } from '@/nav/types'
import { useVisibleNav } from '@/nav/use-visible-nav'

// Split from steps.tsx so that file is pure data - fast refresh needs a module to export either
// components or non-components

export function Note({ children }: { children: ReactNode }) {
  return <p className="text-sm text-muted-foreground">{children}</p>
}

// Names the sections this admin can actually reach, as one line rather than a list - the sidebar
// itself is lit up next to it, so the words only have to confirm what they're looking at
export function YourSections() {
  const { nodes } = useVisibleNav(getNavNodes())
  const groups = nodes.filter(isGroup).map((g) => g.title)

  if (groups.length === 0) {
    return (
      <Note>
        Your account has no sections yet, so there's not much here. Whoever set it up needs to give
        you a role, and then your work appears in this list.
      </Note>
    )
  }
  return (
    <div className="grid gap-2">
      <Note>Everything you can work on is grouped down here. Click a group to open it.</Note>
      <p className="text-sm">
        Yours: <span className="font-medium">{groups.join(', ')}</span>
      </p>
    </div>
  )
}
