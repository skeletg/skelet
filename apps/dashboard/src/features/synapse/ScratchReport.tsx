import { useNavigate, useSearchParams } from 'react-router-dom'
import { InfoIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { AddToBoard } from '@/features/boards/AddToBoard'
import { ChannelRequired } from '@/shared/analytics/ChannelRequired'
import { PERIODS, PeriodPicker } from '@/shared/analytics/PeriodPicker'
import { KINDS, ReportControls, ReportResult, builderOf } from '@/shared/analytics/kinds'
import type { ReportDefinition, ReportKind } from '@/shared/analytics/kinds/definition'

// The report is kept in the URL (?q=<definition>&days=<period>), so a reload, the back button or a pasted link opens
// the same report. A link is only text someone may have edited, so anything this page's builder doesn't make, or whose
// fields aren't the text or lists they should be, opens the page's start instead
export function readReport(params: URLSearchParams, builder: ReportKind, start: ReportKind = builder): ReportDefinition {
  try {
    const d = JSON.parse(params.get('q') ?? '') as ReportDefinition
    const text = (v: unknown) => v == null || typeof v === 'string'
    const fieldsOk = [d?.event, d?.startEvent, d?.returnEvent].every(text) && [d?.steps, d?.filters].every((v) => v == null || Array.isArray(v)) && (d?.steps ?? []).every(text)
    if (d && builderOf(d.kind) === builder && fieldsOk) return d
  } catch {
    // not a report - the start below
  }
  return KINDS[start].start
}

// ScratchReport is a report page: build a report with nothing to save, and put it on a board when it's worth keeping.
// The board's sheet renders the same controls and result
// builder is the kind this page builds in, start the kind it opens on when the URL holds no report
export function ScratchReport({ builder, start, title, description, what, defaultDays = 30 }: { builder: ReportKind; start?: ReportKind; title: string; description: string; what: string; defaultDays?: number }) {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const d = readReport(params, builder, start)
  const days = Number(params.get('days'))
  const sinceDays = (PERIODS as readonly number[]).includes(days) ? days : defaultDays
  const update = (next: ReportDefinition, nextDays = sinceDays) =>
    setParams(
      (p) => {
        p.set('q', JSON.stringify(next)) // as staged - a condition still being typed stays; cleaning is for saving
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
              <AddToBoard channelId={channelId} title={KINDS[d.kind].title(d)} definition={d} />
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
              <ReportResult
                channelId={channelId}
                d={d}
                sinceDays={sinceDays}
                onAddReport={(_title, next) => navigate(`${KINDS[next.kind].page}?q=${encodeURIComponent(JSON.stringify(next))}&days=${sinceDays}`)}
              />
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
