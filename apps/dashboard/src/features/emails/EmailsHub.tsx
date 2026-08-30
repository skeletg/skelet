import { Link, useSearchParams } from 'react-router-dom'
import { PlusIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { EmailsTab } from './EmailsTab'
import { BlocksTab } from './BlocksTab'
import { SlotsTab } from './SlotsTab'

const TABS = ['emails', 'blocks', 'slots'] as const
type TabValue = (typeof TABS)[number]

function isTab(v: string | null): v is TabValue {
  return v != null && (TABS as readonly string[]).includes(v)
}

const TAB_META: Record<TabValue, string> = {
  emails: 'Build an email once, then send it from campaigns and automation flows. Transactional email is delivered by Pulse flows.',
  blocks: 'Reusable content blocks - drop them into any email as a building block.',
  slots: 'Choose which email the store sends for each transactional event. Unassigned slots use the built-in template.',
}

// EmailsHub is the reusable-email library + content-block library under one page with a `?tab=` tab bar - The builder
// editor (/emails/new, /emails/:id) and block editor (/emails/blocks/:slug) are their own routes
export function EmailsHub() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tab = isTab(searchParams.get('tab')) ? (searchParams.get('tab') as TabValue) : 'emails'
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('emails.manage')
  const channelId = effectiveChannelId ?? ''

  function onTabChange(next: string) {
    const params = new URLSearchParams(searchParams)
    params.set('tab', next)
    setSearchParams(params, { replace: true })
  }

  // The primary action lives in the page header (like Flows / Campaigns) and follows the active tab Slots has nothing
  // to create - you bind existing emails there
  const actions =
    isChannel && canManage && tab !== 'slots' ? (
      tab === 'blocks' ? (
        <Button asChild size="sm">
          <Link to="/emails/blocks/new">
            <PlusIcon className="size-4" /> New block
          </Link>
        </Button>
      ) : (
        <Button asChild size="sm">
          <Link to="/emails/new">
            <PlusIcon className="size-4" /> New email
          </Link>
        </Button>
      )
    ) : undefined

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Emails"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">{TAB_META[tab]}</span>}
        actions={actions}
      />
      <DetailPageBody>
        <Tabs value={tab} onValueChange={onTabChange}>
          <div className="px-6 pt-6">
            <TabsList>
              <TabsTrigger value="emails">Emails</TabsTrigger>
              <TabsTrigger value="blocks">Blocks</TabsTrigger>
              <TabsTrigger value="slots">Slots</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="emails" className="mt-0">
            <EmailsTab channelId={channelId} isChannel={isChannel} />
          </TabsContent>
          <TabsContent value="blocks" className="mt-0">
            <BlocksTab channelId={channelId} isChannel={isChannel} />
          </TabsContent>
          <TabsContent value="slots" className="mt-0">
            <SlotsTab channelId={channelId} isChannel={isChannel} />
          </TabsContent>
        </Tabs>
      </DetailPageBody>
    </DetailPageLayout>
  )
}
