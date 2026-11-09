import { useQuery } from '@apollo/client/react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { useDebounced } from '@/shared/hooks/useDebounced'
import type { EmailBlock, EmailStyles } from '@/features/email/blocks'
import { formatBytes, GMAIL_CLIP_BYTES, sizeLevel } from '@/features/email/previewDoc'
import { RenderLibraryPreviewDocument } from './Emails.gql'

// How close the email is to Gmail cutting it off behind "View entire message" - measured on the server's real render,
// fetched once editing pauses
export function EmailSizeMeter({
  channelId,
  subject,
  preheader,
  blocks,
  styles,
}: {
  channelId: string
  subject: string
  preheader: string
  blocks: EmailBlock[]
  styles: EmailStyles
}) {
  const settled = useDebounced(JSON.stringify({ subject, preheader, blocks, styles }), 800)
  const { data, previousData } = useQuery(RenderLibraryPreviewDocument, {
    variables: { channelId, customerId: null, ...(JSON.parse(settled) as { subject: string; preheader: string; blocks: EmailBlock[]; styles: EmailStyles }) },
    fetchPolicy: 'no-cache',
  })
  const html = (data ?? previousData)?.renderLibraryPreview.html
  if (html === undefined) return null
  const bytes = new TextEncoder().encode(html).length
  const level = sizeLevel(bytes)
  const share = Math.min(1, bytes / GMAIL_CLIP_BYTES)
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          role="meter"
          aria-label="Email size"
          aria-valuemin={0}
          aria-valuemax={GMAIL_CLIP_BYTES}
          aria-valuenow={bytes}
          aria-valuetext={`${formatBytes(bytes)} of ${formatBytes(GMAIL_CLIP_BYTES)}`}
          tabIndex={0}
          className="flex h-8 items-center gap-2 rounded-md border bg-background px-2.5 text-xs"
        >
          <span className="text-muted-foreground">Size</span>
          <span className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
            <span
              className={cn('block h-full rounded-full', level === 'over' ? 'bg-destructive' : level === 'warn' ? 'bg-warning' : 'bg-success')}
              style={{ width: `${Math.max(3, share * 100)}%` }}
            />
          </span>
          <span className={cn('tabular-nums', level === 'over' ? 'text-destructive' : level === 'warn' ? 'text-warning' : 'text-foreground')}>
            {formatBytes(bytes)}
          </span>
        </div>
      </TooltipTrigger>
      <TooltipContent side="bottom" className="max-w-64">
        {level === 'over'
          ? `Over Gmail's ${formatBytes(GMAIL_CLIP_BYTES)} limit - Gmail cuts the rest off behind a "View entire message" link, often taking the footer and unsubscribe link with it.`
          : level === 'warn'
            ? `Close to Gmail's ${formatBytes(GMAIL_CLIP_BYTES)} limit. Past it, Gmail cuts the email off behind a "View entire message" link.`
            : `Well under Gmail's ${formatBytes(GMAIL_CLIP_BYTES)} limit, past which it cuts an email off behind a "View entire message" link.`}
      </TooltipContent>
    </Tooltip>
  )
}
