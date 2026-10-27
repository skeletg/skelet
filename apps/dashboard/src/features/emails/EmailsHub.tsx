import { Link, useSearchParams } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { PlusIcon, SparklesIcon } from 'lucide-react'
import { toast } from 'sonner'
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
import { EmailBlocksDocument } from '@/features/email/Email.gql'
import { EmailsTab } from './EmailsTab'
import { BlocksTab } from './BlocksTab'
import { SlotsTab } from './SlotsTab'
import { LibraryEmailsDocument, SeedPulseStarterContentDocument, TransactionalSlotsDocument } from './Emails.gql'
import { useConfirmDialog } from '@/shared/dialogs/useConfirmDialog'

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
  const [confirm, confirmDialog] = useConfirmDialog()
  const [searchParams, setSearchParams] = useSearchParams()
  const tab = isTab(searchParams.get('tab')) ? (searchParams.get('tab') as TabValue) : 'emails'
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('emails.manage')
  const canSeed = canManage && has('flows.manage') // the seed writes flows as well as emails
  const channelId = effectiveChannelId ?? ''

  function onTabChange(next: string) {
    const params = new URLSearchParams(searchParams)
    params.set('tab', next)
    setSearchParams(params, { replace: true })
  }

  // Starter content is asked for, never automatic - an active order_placed flow moves receipts into the builder's emails
  // Idempotent, asking again adds only what is missing
  const [seedStarter, { loading: seeding }] = useMutation(SeedPulseStarterContentDocument, {
    refetchQueries: [
      { query: LibraryEmailsDocument, variables: { channelId, groupSlug: null } },
      { query: EmailBlocksDocument, variables: { channelId } },
      { query: TransactionalSlotsDocument, variables: { channelId } },
    ],
    onCompleted: (data) => {
      const r = data.seedPulseStarterContent
      const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`
      toast.success(
        r.blocks + r.emails + r.flows === 0
          ? 'This channel already has the starter content'
          : `Added ${plural(r.emails, 'email')}, ${plural(r.flows, 'flow')} and ${plural(r.blocks, 'block')}`,
      )
    },
    onError: (err) => toast.error(err.message),
  })

  // The primary action lives in the page header and follows the active tab - Slots has nothing to create
  const actions =
    isChannel && canManage ? (
      <div className="flex items-center gap-2">
        {canSeed && (
          <Button
            size="sm"
            variant="outline"
            disabled={seeding}
            onClick={() =>
              confirm({
                title: 'Add the starter content to this channel?',
                description:
                  'It creates an editable email for every transactional slot and an active flow that sends it, so receipts and account mail go out from these emails instead of the built-in templates. Anything the channel already has is left alone.',
                confirmLabel: 'Add starter content',
                onConfirm: () => void seedStarter({ variables: { channelId } }),
              })
            }
          >
            <SparklesIcon className="size-4" /> Add starter content
          </Button>
        )}
        {tab === 'blocks' ? (
          <Button asChild size="sm">
            <Link to="/emails/blocks/new">
              <PlusIcon className="size-4" /> New block
            </Link>
          </Button>
        ) : tab === 'emails' ? (
          <Button asChild size="sm">
            <Link to="/emails/new">
              <PlusIcon className="size-4" /> New email
            </Link>
          </Button>
        ) : null}
      </div>
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
      {confirmDialog}
    </DetailPageLayout>
  )
}
