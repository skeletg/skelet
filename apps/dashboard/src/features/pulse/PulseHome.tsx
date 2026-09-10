import { useMemo, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { ClipboardListIcon, MailIcon, MegaphoneIcon, SendIcon, ShieldOffIcon, WorkflowIcon, type LucideIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { EmailCampaignsDocument } from '@/features/campaigns/Campaigns.gql'
import { FlowsDocument } from '@/features/flows/Flows.gql'
import { LibraryEmailsDocument } from '@/features/emails/Emails.gql'
import { PulseEngagementDocument } from './Pulse.signals.gql'

const ACCENT = '#6366f1'

type NavCard = { to: string; title: string; description: string; icon: LucideIcon }
const CARDS: NavCard[] = [
  { to: '/emails', title: 'Emails', description: 'Emails, templates & blocks in one place - build them, wire transactional slots, reuse content.', icon: MailIcon },
  { to: '/campaigns', title: 'Campaigns', description: 'One-off broadcasts to everyone, a group or a segment - now or scheduled.', icon: MegaphoneIcon },
  { to: '/flows', title: 'Flows', description: 'Automated, event-triggered journeys - welcome, win-back, abandoned cart…', icon: WorkflowIcon },
  { to: '/forms', title: 'Signup forms', description: 'Popups & embedded forms that grow your list - feed a welcome flow.', icon: ClipboardListIcon },
  { to: '/emails/suppressions', title: 'Suppression list', description: 'Addresses blocked from marketing - bounces, complaints, manual entries.', icon: ShieldOffIcon },
]

const STATUS_LABEL: Record<string, string> = { draft: 'Draft', scheduled: 'Scheduled', sending: 'Sending', sent: 'Sent', cancelled: 'Cancelled' }
const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = { draft: 'outline', scheduled: 'secondary', sending: 'default', sent: 'default', cancelled: 'outline' }
const FLOW_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = { active: 'default', paused: 'secondary', draft: 'outline' }
const TRIGGER_LABEL: Record<string, string> = {
  'order.placed': 'Order placed',
  'order.cancelled': 'Order cancelled',
  'order.refunded': 'Order refunded',
  'customer.registered': 'Customer registered',
  'cart.abandoned': 'Abandoned cart',
  abandoned_cart: 'Abandoned cart',
}
const triggerLabel = (t: string) => TRIGGER_LABEL[t] ?? t.replace(/[._]/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())

const nfmt = (n: number) => n.toLocaleString()
const pctStr = (x: number | null) => (x == null ? '-' : `${Math.round(x * 100)}%`)
const fmtMoney = (cents: number, currency: string) => (cents / 100).toLocaleString(undefined, { style: 'currency', currency: currency || 'USD' })
function fmtDateTime(iso?: string | null): string {
  if (!iso) return '-'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}
function fmtDate(iso?: string | null): string {
  if (!iso) return '-'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString(undefined, { dateStyle: 'medium' })
}

export function PulseHome() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const skip = !effectiveChannelId

  const { data: campData } = useQuery(EmailCampaignsDocument, { variables: { channelId }, skip, fetchPolicy: 'cache-and-network' })
  const { data: flowData } = useQuery(FlowsDocument, { variables: { channelId }, skip, fetchPolicy: 'cache-and-network' })
  const { data: emailData } = useQuery(LibraryEmailsDocument, { variables: { channelId }, skip, fetchPolicy: 'cache-and-network' })
  // Spine-backed engagement rollup (all sources: campaigns + flows + transactional), last 30 days
  const { data: engData } = useQuery(PulseEngagementDocument, { variables: { channelId, sinceDays: 30 }, skip, fetchPolicy: 'cache-and-network' })
  const eng = engData?.pulseEngagement

  const s = useMemo(() => {
    const campaigns = campData?.emailCampaigns ?? []
    const flows = flowData?.flows ?? []
    const emailCount = (emailData?.libraryEmails ?? []).length

    const sent = campaigns.reduce((a, c) => a + (c.sentCount ?? 0), 0)
    const failed = campaigns.reduce((a, c) => a + (c.failedCount ?? 0), 0)
    const skipped = campaigns.reduce((a, c) => a + (c.skippedCount ?? 0), 0)
    const recipients = campaigns.reduce((a, c) => a + (c.totalRecipients ?? 0), 0)
    const opened = campaigns.reduce((a, c) => a + (c.openedCount ?? 0), 0)
    const clicked = campaigns.reduce((a, c) => a + (c.clickedCount ?? 0), 0)
    const deliveryRate = sent + failed > 0 ? sent / (sent + failed) : null
    const openRate = sent > 0 ? opened / sent : null
    const clickRate = sent > 0 ? clicked / sent : null

    const byStatus: Record<string, number> = {}
    for (const c of campaigns) byStatus[c.status] = (byStatus[c.status] ?? 0) + 1

    const byFlowStatus: Record<string, number> = {}
    for (const f of flows) byFlowStatus[f.status] = (byFlowStatus[f.status] ?? 0) + 1

    const upcoming = campaigns
      .filter((c) => c.status === 'scheduled' && c.scheduledFor)
      .sort((a, b) => new Date(a.scheduledFor as string).getTime() - new Date(b.scheduledFor as string).getTime())
      .slice(0, 5)

    const topCampaigns = campaigns
      .filter((c) => (c.sentCount ?? 0) > 0)
      .sort((a, b) => (b.sentCount ?? 0) - (a.sentCount ?? 0))
      .slice(0, 6)
      .map((c) => ({ name: c.name, sent: c.sentCount ?? 0 }))

    const recent = campaigns
      .filter((c) => c.sentAt || c.status === 'sent' || c.status === 'sending')
      .sort((a, b) => new Date(b.sentAt ?? b.updatedAt).getTime() - new Date(a.sentAt ?? a.updatedAt).getTime())
      .slice(0, 8)
      .map((c) => {
        const cs = c.sentCount ?? 0
        return {
          id: c.id,
          name: c.name,
          status: c.status,
          recipients: c.totalRecipients ?? 0,
          sent: cs,
          delivery: cs + (c.failedCount ?? 0) > 0 ? cs / (cs + (c.failedCount ?? 0)) : null,
          open: cs > 0 ? (c.openedCount ?? 0) / cs : null,
          click: cs > 0 ? (c.clickedCount ?? 0) / cs : null,
          sentAt: c.sentAt,
        }
      })

    const flowRows = [...flows]
      .sort((a, b) => Number(b.status === 'active') - Number(a.status === 'active') || a.name.localeCompare(b.name))
      .slice(0, 8)

    return {
      campaignCount: campaigns.length,
      flowCount: flows.length,
      emailCount,
      sent,
      failed,
      skipped,
      recipients,
      opened,
      clicked,
      deliveryRate,
      openRate,
      clickRate,
      byStatus,
      byFlowStatus,
      activeFlows: byFlowStatus['active'] ?? 0,
      sentCampaigns: byStatus['sent'] ?? 0,
      upcoming,
      topCampaigns,
      recent,
      flowRows,
    }
  }, [campData, flowData, emailData])

  // Send → click funnel, scaled to the widest step (targeted recipients)
  const funnelBase = Math.max(s.recipients, s.sent, s.opened, s.clicked, 1)
  const funnel = [
    { label: 'Targeted', value: s.recipients, rate: null as number | null, note: 'recipients' },
    { label: 'Delivered', value: s.sent, rate: s.recipients > 0 ? s.sent / s.recipients : null, note: 'of targeted' },
    { label: 'Opened', value: s.opened, rate: s.sent > 0 ? s.opened / s.sent : null, note: 'of delivered' },
    { label: 'Clicked', value: s.clicked, rate: s.sent > 0 ? s.clicked / s.sent : null, note: 'of delivered' },
  ]

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Pulse"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">Email marketing &amp; automation - build it, send it, automate it. Stats cover the last 30 days.</span>}
      />
      <DetailPageBody>
        <DetailPageSection title="Explore" description="Everything Pulse does, in one place.">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {CARDS.map((c) => (
              <Link key={c.to} to={c.to} className="group flex items-start gap-3 rounded-lg border p-4 transition hover:border-primary/40 hover:bg-muted/40">
                <span className="grid size-9 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
                  <c.icon className="size-5" />
                </span>
                <span className="min-w-0">
                  <span className="block font-medium group-hover:text-primary">{c.title}</span>
                  <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{c.description}</span>
                </span>
              </Link>
            ))}
          </div>
        </DetailPageSection>

        {!isChannel && (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel for analytics</AlertTitle>
              <AlertDescription>Pulse stats are per channel - pick one in the header scope picker. You can still jump into any area above.</AlertDescription>
            </Alert>
          </DetailPageSection>
        )}

        {isChannel && (
          <>
            <DetailPageSection title="At a glance">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <StatTile icon={MailIcon} value={nfmt(s.emailCount)} label="Emails" sub="in the library" />
                <StatTile icon={WorkflowIcon} value={nfmt(s.activeFlows)} label="Active flows" sub={`of ${nfmt(s.flowCount)} total`} />
                <StatTile icon={MegaphoneIcon} value={nfmt(s.campaignCount)} label="Campaigns" sub={`${nfmt(s.sentCampaigns)} sent`} />
                <StatTile icon={SendIcon} value={nfmt(s.sent)} label="Emails delivered" sub={s.deliveryRate != null ? `${pctStr(s.deliveryRate)} delivery rate` : 'across all campaigns'} />
              </div>
            </DetailPageSection>

            <DetailPageSection title="From send to click" description="How every message fares from the moment it's queued through to a click.">
              <div className="grid gap-4 lg:grid-cols-2">
                <Panel title="Funnel">
                  {s.recipients === 0 ? (
                    <Empty>No emails sent yet.</Empty>
                  ) : (
                    <div className="space-y-2.5 py-1">
                      {funnel.map((f) => (
                        <div key={f.label} className="flex items-center gap-3 text-sm">
                          <span className="w-20 shrink-0 text-muted-foreground">{f.label}</span>
                          <span className="h-2.5 min-w-16 flex-1 overflow-hidden rounded-full bg-muted">
                            <span className="block h-full rounded-full bg-primary" style={{ width: `${(f.value / funnelBase) * 100}%` }} />
                          </span>
                          <span className="w-14 shrink-0 text-right tabular-nums">{nfmt(f.value)}</span>
                          <span className="w-24 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{f.rate != null ? `${pctStr(f.rate)} ${f.note}` : ''}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </Panel>

                <Panel title="Delivery">
                  <DeliveryBar sent={s.sent} failed={s.failed} skipped={s.skipped} />
                  <div className="mt-4 grid grid-cols-3 gap-2 text-center">
                    <Metric label="Delivered" value={nfmt(s.sent)} dotClass="bg-emerald-500" />
                    <Metric label="Failed" value={nfmt(s.failed)} dotClass="bg-rose-500" />
                    <Metric label="Skipped" value={nfmt(s.skipped)} dotClass="bg-muted-foreground/40" />
                  </div>
                  <p className="mt-3 text-xs text-muted-foreground">{nfmt(s.recipients)} recipients targeted across {nfmt(s.campaignCount)} campaigns.</p>
                </Panel>
              </div>
            </DetailPageSection>

            <DetailPageSection title="Engagement">
              <div className="grid gap-4 lg:grid-cols-3">
                <Panel title="Open & click rate">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <div className="text-3xl font-semibold tabular-nums">{pctStr(s.openRate)}</div>
                      <p className="mt-1 text-xs text-muted-foreground">{nfmt(s.opened)} opens</p>
                    </div>
                    <div>
                      <div className="text-3xl font-semibold tabular-nums">{pctStr(s.clickRate)}</div>
                      <p className="mt-1 text-xs text-muted-foreground">{nfmt(s.clicked)} clicks</p>
                    </div>
                  </div>
                  <p className="mt-3 text-xs text-muted-foreground">Opens rely on a tracking pixel - undercounted by image-blockers, inflated by privacy proxies. Clicks are the reliable signal.</p>
                </Panel>

                <Panel title="Opens - real vs machine">
                  {eng ? (
                    <>
                      <div className="text-3xl font-semibold tabular-nums">{nfmt(eng.opensHuman)}</div>
                      <p className="mt-1 text-xs text-muted-foreground">real opens</p>
                      <div className="mt-3 space-y-1 text-xs text-muted-foreground">
                        <div className="flex justify-between"><span>Raw opens</span><span className="tabular-nums">{nfmt(eng.opensRaw)}</span></div>
                        <div className="flex justify-between"><span>Machine / prefetch</span><span className="tabular-nums">{nfmt(Math.max(0, eng.opensRaw - eng.opensHuman))}</span></div>
                        <div className="flex justify-between"><span>Clicks</span><span className="tabular-nums">{nfmt(eng.clicks)}</span></div>
                      </div>
                    </>
                  ) : (
                    <Empty>No spine data yet.</Empty>
                  )}
                </Panel>

                <Panel title="Attributed revenue">
                  {eng ? (
                    <>
                      {eng.revenue.length === 0 ? (
                        <div className="text-3xl font-semibold tabular-nums">-</div>
                      ) : (
                        eng.revenue.map((r) => (
                          <div key={r.currency || 'na'} className="text-2xl font-semibold tabular-nums">{fmtMoney(r.cents, r.currency)}</div>
                        ))
                      )}
                      <p className="mt-1 text-xs text-muted-foreground">{nfmt(eng.conversions)} orders credited to email (last-touch, 30d)</p>
                    </>
                  ) : (
                    <Empty>No spine data yet.</Empty>
                  )}
                </Panel>
              </div>
            </DetailPageSection>

            <DetailPageSection title="Campaigns" description="Volume and per-campaign performance.">
              <div className="grid gap-4 lg:grid-cols-2">
                <Panel title="Top campaigns by sends">
                  {s.topCampaigns.length === 0 ? (
                    <Empty>No campaigns sent yet.</Empty>
                  ) : (
                    <div className="h-56">
                      <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={s.topCampaigns} margin={{ top: 8, right: 12, bottom: 0, left: -16 }}>
                          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                          <XAxis dataKey="name" tick={{ fontSize: 11 }} tickFormatter={(v: string) => (v.length > 12 ? `${v.slice(0, 11)}…` : v)} interval={0} />
                          <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={40} />
                          <Tooltip content={<ChartTooltipContent />} />
                          <Bar dataKey="sent" name="Delivered" fill={ACCENT} radius={[4, 4, 0, 0]} />
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                  )}
                </Panel>

                <Panel title="Recent broadcasts">
                  {s.recent.length === 0 ? (
                    <Empty>No campaigns sent yet.</Empty>
                  ) : (
                    <div className="-mx-3 -mb-3 overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-y text-left text-xs uppercase tracking-wider text-muted-foreground">
                            <th className="px-3 py-2">Campaign</th>
                            <th className="px-3 py-2 text-right">Sent</th>
                            <th className="px-3 py-2 text-right">Open</th>
                            <th className="px-3 py-2 text-right">Click</th>
                          </tr>
                        </thead>
                        <tbody>
                          {s.recent.map((c) => (
                            <tr key={c.id} className="border-b last:border-0 hover:bg-muted/40">
                              <td className="px-3 py-2">
                                <Link to={`/campaigns/${c.id}`} className="font-medium hover:underline">{c.name}</Link>
                                <div className="text-xs text-muted-foreground">{fmtDate(c.sentAt)}</div>
                              </td>
                              <td className="px-3 py-2 text-right tabular-nums">{nfmt(c.sent)}</td>
                              <td className="px-3 py-2 text-right tabular-nums">{pctStr(c.open)}</td>
                              <td className="px-3 py-2 text-right tabular-nums">{pctStr(c.click)}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </Panel>
              </div>
            </DetailPageSection>

            {eng && eng.topLinks.length > 0 && (
              <DetailPageSection title="Top links" description="Most-clicked URLs across all emails (last 30 days).">
                <Panel title="By clicks">
                  <ul className="space-y-1.5">
                    {eng.topLinks.slice(0, 8).map((l) => (
                      <li key={l.url} className="flex items-center justify-between gap-3 text-sm">
                        <span className="min-w-0 truncate text-muted-foreground">{l.url}</span>
                        <span className="shrink-0 font-medium tabular-nums">{nfmt(l.clicks)}</span>
                      </li>
                    ))}
                  </ul>
                </Panel>
              </DetailPageSection>
            )}

            <DetailPageSection title="Automation" description={`${nfmt(s.activeFlows)} active · ${nfmt(s.byFlowStatus['paused'] ?? 0)} paused · ${nfmt(s.byFlowStatus['draft'] ?? 0)} draft`}>
              <Panel title="Flows">
                {s.flowRows.length === 0 ? (
                  <Empty>No flows yet. <Link to="/flows" className="text-primary hover:underline">Create one</Link>.</Empty>
                ) : (
                  <div className="-mx-3 -mb-3 overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-y text-left text-xs uppercase tracking-wider text-muted-foreground">
                          <th className="px-3 py-2">Flow</th>
                          <th className="px-3 py-2">Trigger</th>
                          <th className="px-3 py-2">Status</th>
                        </tr>
                      </thead>
                      <tbody>
                        {s.flowRows.map((f) => (
                          <tr key={f.id} className="border-b last:border-0 hover:bg-muted/40">
                            <td className="px-3 py-2"><Link to={`/flows/${f.id}`} className="font-medium hover:underline">{f.name}</Link></td>
                            <td className="px-3 py-2 text-muted-foreground">{triggerLabel(f.triggerType)}</td>
                            <td className="px-3 py-2"><Badge variant={FLOW_VARIANT[f.status] ?? 'outline'} className="capitalize">{f.status}</Badge></td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Panel>
            </DetailPageSection>

            <DetailPageSection title="Pipeline">
              <div className="grid gap-4 lg:grid-cols-2">
                <Panel title="Upcoming sends">
                  {s.upcoming.length === 0 ? (
                    <Empty>Nothing scheduled.</Empty>
                  ) : (
                    <ul className="divide-y">
                      {s.upcoming.map((c) => (
                        <li key={c.id} className="flex items-center justify-between gap-3 py-2">
                          <Link to={`/campaigns/${c.id}`} className="min-w-0 truncate text-sm font-medium hover:underline">{c.name}</Link>
                          <span className="shrink-0 text-xs text-muted-foreground">{fmtDateTime(c.scheduledFor)}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </Panel>

                <Panel title="Campaigns by status">
                  {s.campaignCount === 0 ? (
                    <Empty>No campaigns yet.</Empty>
                  ) : (
                    <div className="flex flex-wrap gap-2">
                      {Object.entries(s.byStatus).map(([status, count]) => (
                        <Badge key={status} variant={STATUS_VARIANT[status] ?? 'outline'} className="gap-1.5">
                          {STATUS_LABEL[status] ?? status}
                          <span className="font-mono">{count}</span>
                        </Badge>
                      ))}
                    </div>
                  )}
                </Panel>
              </div>
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function StatTile({ icon: Icon, value, label, sub }: { icon: LucideIcon; value: string; label: string; sub?: string }) {
  return (
    <div className="flex items-center gap-3 rounded border p-4">
      <span className="grid size-10 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
        <Icon className="size-5" />
      </span>
      <div className="min-w-0">
        <div className="text-2xl font-semibold leading-none tabular-nums">{value}</div>
        <div className="mt-1 text-xs font-medium">{label}</div>
        {sub && <div className="text-[11px] text-muted-foreground">{sub}</div>}
      </div>
    </div>
  )
}

function DeliveryBar({ sent, failed, skipped }: { sent: number; failed: number; skipped: number }) {
  const total = sent + failed + skipped
  const pct = (n: number) => (total > 0 ? `${(n / total) * 100}%` : '0%')
  if (total === 0) return <div className="h-2.5 w-full rounded-full bg-muted" />
  return (
    <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
      <div className="bg-emerald-500" style={{ width: pct(sent) }} />
      <div className="bg-rose-500" style={{ width: pct(failed) }} />
      <div className="bg-muted-foreground/40" style={{ width: pct(skipped) }} />
    </div>
  )
}

function Metric({ label, value, dotClass }: { label: string; value: string; dotClass: string }) {
  return (
    <div>
      <div className="text-lg font-semibold tabular-nums">{value}</div>
      <div className="flex items-center justify-center gap-1 text-[11px] text-muted-foreground">
        <span className={`size-2 rounded-full ${dotClass}`} />
        {label}
      </div>
    </div>
  )
}

// A bordered panel with a header strip - the flat skelet alternative to a Card
function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
      <div className="p-3">{children}</div>
    </div>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>
}
