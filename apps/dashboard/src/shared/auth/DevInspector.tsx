import { useMemo } from 'react'
import { useLocation } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { ExternalLinkIcon, ShieldAlertIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { scopePermissionForPath } from '@/nav/scope-permission'
import { useConfigScope } from '@/lib/config-scope'
import { useDevRequests } from '@/lib/dev-monitor'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { MeDocument } from './Me.gql'
import { useAuth } from './auth'
import { STOREFRONT_ORIGIN } from '@/lib/app-origins'

// Dev-only inspector sheet: the resolved truth behind permission /
// channel / locale debugging - Everything here is read-only and comes
// from data the dashboard already has (me, channels, header scope) or
// from the dev request monitor (X-DB-Queries headers + denial
// extensions) - Opened from the dev identity switcher in the sidebar

const BACKEND_URL = import.meta.env.VITE_BACKEND_URL ?? 'http://localhost:8080'
const JAEGER_URL = import.meta.env.VITE_JAEGER_URL ?? 'http://localhost:16686'

const TOOL_LINKS: { label: string; href: string; hint: string }[] = [
  { label: 'Mailpit', href: 'http://localhost:8025', hint: 'caught emails' },
  { label: 'Jaeger', href: JAEGER_URL, hint: 'traces' },
  { label: 'Garage', href: 'http://localhost:9901', hint: 'object storage console' },
  { label: 'Webhook tester', href: 'http://localhost:8081', hint: 'outbound webhooks' },
  { label: 'Admin playground', href: `${BACKEND_URL}/playground/admin`, hint: 'GraphQL' },
  { label: 'Front playground', href: `${BACKEND_URL}/playground/front`, hint: 'GraphQL' },
]

export function DevInspector({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { auth } = useAuth()
  const location = useLocation()
  const { scope, selectedChannel, effectiveLocale, restrictedToChannels } = useConfigScope()
  const requests = useDevRequests()

  // Full channel list for id → name resolution + preview links - Errors
  // (scoped admin) degrade to short ids; the section still renders
  const channelsQuery = useQuery(ChannelsDocument, { errorPolicy: 'ignore', fetchPolicy: 'cache-first' })
  const channels = useMemo(() => channelsQuery.data?.channels ?? [], [channelsQuery.data])
  const channelName = (id: string | null) => {
    if (!id) return 'all channels'
    return channels.find((c) => c.id === id)?.name ?? `${id.slice(0, 8)}…`
  }

  const meQuery = useQuery(MeDocument, { fetchPolicy: 'cache-first' })
  const me = meQuery.data?.me

  const pagePermission = scopePermissionForPath(location.pathname)
  const denials = requests.filter((r) => r.deniedPermission)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[480px] overflow-y-auto sm:max-w-[480px]">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2 text-base">
            Dev inspector
            <Badge variant="outline" className="border-amber-500/60 font-mono text-[10px] uppercase text-amber-500">
              dev
            </Badge>
          </SheetTitle>
          <SheetDescription>
            Resolved permission, scope, and request state for the current identity.
          </SheetDescription>
        </SheetHeader>

        <div className="grid gap-5 px-4 pb-6 text-sm">
          {/* ---- Identity & grants ---- */}
          <section className="grid gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Identity
            </h3>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{auth?.admin.email ?? 'not signed in'}</span>
              {auth?.admin.isSuperuser && <Badge variant="destructive">superuser</Badge>}
            </div>
            {auth?.admin.isSuperuser ? (
              <p className="text-xs text-muted-foreground">
                Superusers bypass every permission and channel check - grants below are irrelevant.
              </p>
            ) : me && me.roles.length > 0 ? (
              <div className="rounded-md border">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="border-b text-left text-muted-foreground">
                      <th className="px-2 py-1 font-medium">Permission</th>
                      <th className="px-2 py-1 font-medium">Channel</th>
                      <th className="px-2 py-1 font-medium">Locale</th>
                    </tr>
                  </thead>
                  <tbody>
                    {me.roles.flatMap((role) =>
                      role.scopes.map((s, i) => (
                        <tr key={`${role.id}:${i}`} className="border-b last:border-0">
                          <td className="px-2 py-1 font-mono">{s.permission.slug}</td>
                          <td className="px-2 py-1">{channelName(s.channelId ?? null)}</td>
                          <td className="px-2 py-1">{s.locale ?? 'all'}</td>
                        </tr>
                      )),
                    )}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">No role grants.</p>
            )}
          </section>

          {/* ---- Scope resolution ---- */}
          <section className="grid gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Scope resolution
            </h3>
            <dl className="grid grid-cols-[140px_1fr] gap-y-1 text-xs">
              <dt className="text-muted-foreground">Page</dt>
              <dd className="font-mono">{location.pathname}</dd>
              <dt className="text-muted-foreground">Page permission</dt>
              <dd className="font-mono">{pagePermission ?? '- (unfiltered)'}</dd>
              <dt className="text-muted-foreground">Header mode</dt>
              <dd>{scope.mode}</dd>
              <dt className="text-muted-foreground">Channel</dt>
              <dd>{selectedChannel ? selectedChannel.name : scope.channelId ? channelName(scope.channelId) : '-'}</dd>
              <dt className="text-muted-foreground">Locale header sent</dt>
              <dd className="font-mono">{effectiveLocale || '- (backend default)'}</dd>
              <dt className="text-muted-foreground">Channel-restricted</dt>
              <dd>{restrictedToChannels ? 'yes (Global disabled)' : 'no'}</dd>
            </dl>
          </section>

          {/* ---- Recent denials ---- */}
          <section className="grid gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Recent denials
            </h3>
            {denials.length === 0 ? (
              <p className="text-xs text-muted-foreground">No FORBIDDEN responses recorded this session.</p>
            ) : (
              <ul className="grid gap-1">
                {denials.slice(0, 8).map((d) => (
                  <li key={d.id} className="flex items-start gap-1.5 rounded-md border border-destructive/30 px-2 py-1 text-xs">
                    <ShieldAlertIcon className="mt-0.5 size-3 shrink-0 text-destructive" />
                    <span>
                      <span className="font-mono">{d.operation}</span> denied -
                      needs <span className="font-mono">{d.deniedPermission}</span>
                      {d.deniedChannelId ? <> on {channelName(d.deniedChannelId)}</> : null}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>

          {/* ---- Recent requests ---- */}
          <section className="grid gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Recent requests
            </h3>
            {requests.length === 0 ? (
              <p className="text-xs text-muted-foreground">No requests recorded yet.</p>
            ) : (
              <div className="rounded-md border">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="border-b text-left text-muted-foreground">
                      <th className="px-2 py-1 font-medium">Operation</th>
                      <th className="px-2 py-1 text-right font-medium">DB queries</th>
                      <th className="px-2 py-1 font-medium">Trace</th>
                    </tr>
                  </thead>
                  <tbody>
                    {requests.slice(0, 12).map((r) => (
                      <tr key={r.id} className="border-b last:border-0">
                        <td className="max-w-44 truncate px-2 py-1 font-mono">{r.operation}</td>
                        <td className={`px-2 py-1 text-right font-mono ${r.dbQueries !== null && r.dbQueries > 50 ? 'text-destructive' : ''}`}>
                          {r.dbQueries ?? '-'}
                        </td>
                        <td className="px-2 py-1">
                          {r.traceId ? (
                            <a
                              href={`${JAEGER_URL}/trace/${r.traceId}`}
                              target="_blank"
                              rel="noreferrer"
                              className="inline-flex items-center gap-0.5 text-primary hover:underline"
                            >
                              open <ExternalLinkIcon className="size-2.5" />
                            </a>
                          ) : (
                            '-'
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          {/* ---- Storefront preview ---- */}
          <section className="grid gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Storefront preview
            </h3>
            <p className="text-xs text-muted-foreground">
              Opens each channel at its own public URL, in the chosen locale. Both come from
              the address now, not cookies: the channel is the deployment, the locale is a
              path prefix. Channels with no public URL set have nothing to open.
            </p>
            <div className="grid gap-1">
              {channels.map((ch) => {
                // Its own address, or the configured fallback for whichever channel this dev
                // storefront happens to be pinned to - a channel with neither can't be opened
                const base = ch.publicUrl || (ch.isDefault ? STOREFRONT_ORIGIN : '')
                return (
                  <div key={ch.id} className="flex flex-wrap items-center gap-1.5 text-xs">
                    <span className="w-28 truncate text-muted-foreground">{ch.name}</span>
                    {!base && <span className="text-muted-foreground">no public URL set</span>}
                    {base &&
                      (ch.locales.length ? ch.locales.map((l) => l.code) : [ch.defaultLocale]).map((code) => (
                        <a
                          key={code}
                          href={code === ch.defaultLocale ? base : `${base}/${encodeURIComponent(code)}`}
                          target="_blank"
                          rel="noreferrer"
                          className="rounded border px-1.5 py-0.5 font-mono hover:bg-accent"
                        >
                          {code}
                        </a>
                      ))}
                  </div>
                )
              })}
            </div>
          </section>

          {/* ---- Tools ---- */}
          <section className="grid gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Local tools
            </h3>
            <div className="grid grid-cols-2 gap-1.5">
              {TOOL_LINKS.map((t) => (
                <a
                  key={t.label}
                  href={t.href}
                  target="_blank"
                  rel="noreferrer"
                  className="flex items-center justify-between rounded-md border px-2 py-1.5 text-xs hover:bg-accent"
                >
                  <span>
                    <span className="block font-medium">{t.label}</span>
                    <span className="text-muted-foreground">{t.hint}</span>
                  </span>
                  <ExternalLinkIcon className="size-3 text-muted-foreground" />
                </a>
              ))}
            </div>
          </section>
        </div>
      </SheetContent>
    </Sheet>
  )
}
