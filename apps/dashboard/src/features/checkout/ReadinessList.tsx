import { Link } from 'react-router-dom'
import { CircleAlertIcon, CircleCheckIcon, TriangleAlertIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { TABS, fixTarget, type TabId } from './labels'
import type { Readiness } from './types'

export type GoTo = (tab: TabId, params?: { option?: string }) => void

// A setup checklist - passing checks say what satisfies them, failing ones link to where they're fixed. A failing advisory check
// is a warning: it doesn't hold the option back, so it reads as one rather than as a to-do
export function ReadinessList({
  checks,
  optionId,
  onGo,
  hideFixFor,
}: {
  checks: readonly Readiness[]
  optionId?: string
  onGo: GoTo
  // The tab the list is already shown on - a link back to it would go nowhere
  hideFixFor?: TabId
}) {
  if (!checks.length) return null
  return (
    <ul className="divide-y rounded-md border">
      {checks.map((check) => {
        const target = fixTarget(check, optionId)
        const sameTab = 'tab' in target && target.tab === hideFixFor && target.optionId === optionId
        return (
          <li key={check.code + check.label} className="flex flex-wrap items-start gap-3 p-3">
            {check.ok ? (
              <CircleCheckIcon className="mt-0.5 size-4 shrink-0 text-success" aria-label="Done" />
            ) : check.advisory ? (
              <TriangleAlertIcon className="mt-0.5 size-4 shrink-0 text-warning" aria-label="Warning" />
            ) : (
              <CircleAlertIcon className="mt-0.5 size-4 shrink-0 text-warning" aria-label="To do" />
            )}
            <div className="grid min-w-0 flex-1 gap-0.5">
              <span className="text-sm font-medium">
                {check.label}
                {!check.ok && check.advisory ? <span className="ml-2 text-xs font-normal text-warning">Warning - won't block going live</span> : null}
              </span>
              {check.detail ? <span className="text-sm text-muted-foreground">{check.detail}</span> : null}
            </div>
            {!check.ok && !sameTab && (
              'href' in target ? (
                <Button asChild size="sm" variant="outline">
                  <Link to={target.href}>{target.label}</Link>
                </Button>
              ) : (
                <Button size="sm" variant="outline" onClick={() => onGo(target.tab, { option: target.optionId })}>
                  Fix in {TABS.find((t) => t.id === target.tab)?.label}
                </Button>
              )
            )}
          </li>
        )
      })}
    </ul>
  )
}
