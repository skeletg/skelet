import { useSearchParams } from 'react-router-dom'
import { InfoIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { AddToBoard } from '@/features/boards/AddToBoard'
import { ChannelRequired } from '@/shared/analytics/ChannelRequired'
import { PeriodPicker } from '@/shared/analytics/PeriodPicker'
import { PERIODS } from '@/shared/analytics/PeriodPicker'
import { REPORTS, cleanDefinition, type ReportDefinition, type ReportKind } from '@/features/boards/report'
import { ReportControls, ReportResult, suggestTitle } from '@/shared/analytics/kinds'

// The report is kept in the URL (?q=<definition>&days=<period>), so a reload, the back button or a pasted link opens
// the same report - anything that isn't one of this page's kinds falls back to its default
export function readReport(params: URLSearchParams, kinds: ReportKind[]): ReportDefinition {
  try {
    const d = JSON.parse(params.get('q') ?? '') as ReportDefinition
    if (d && kinds.includes(d.kind)) return d
  } catch {
    // not a report - the default below
  }
  return REPORTS[kinds[0]].start
}

// ScratchReport is a report page: build a report with nothing to save, and put it on a board when it's worth keeping.
// The board's sheet renders the same controls and result
export function ScratchReport({ kinds, title, description, what, defaultDays = 30 }: { kinds: ReportKind[]; title: string; description: string; what: string; defaultDays?: number }) {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [params, setParams] = useSearchParams()
  const d = readReport(params, kinds)
  const days = Number(params.get('days'))
  const sinceDays = (PERIODS as readonly number[]).includes(days) ? days : defaultDays
  const update = (next: ReportDefinition, nextDays = sinceDays) =>
    setParams(
      (p) => {
        p.set('q', JSON.stringify(cleanDefinition(next)))
        p.set('days', String(nextDays))
        return p
      },
      { replace: true },
    )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={title}
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        actions={
          isChannel ? (
            <div className="flex items-center gap-3">
              <PeriodPicker value={sinceDays} onChange={(n) => update(d, n)} />
              <AddToBoard channelId={channelId} title={suggestTitle(d)} definition={d} />
            </div>
          ) : undefined
        }
        meta={
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            {description}
            <Tooltip>
              <TooltipTrigger asChild>
                <button type="button" aria-label="How people are counted" className="rounded text-muted-foreground hover:text-foreground">
                  <InfoIcon className="size-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>A person is resolved across their anonymous and signed-in activity.</TooltipContent>
            </Tooltip>
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <ChannelRequired what={what} />
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection title="Query">
              <ReportControls channelId={channelId} value={d} onChange={(next) => update(next)} />
            </DetailPageSection>
            <DetailPageSection title="Result">
              <ReportResult channelId={channelId} d={d} sinceDays={sinceDays} />
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
