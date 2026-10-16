import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { Link, useSearchParams } from 'react-router-dom'
import { DataTable } from '@/components/data-table'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SettingsContent } from '@/features/settings/SettingsContent'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { useConfigDrafts } from '@/features/settings/useConfigDrafts'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { PaymentProvidersDocument, type PaymentProvidersQuery } from './Payments.gql'

type Provider = PaymentProvidersQuery['paymentProviders'][number]

export function PaymentProvidersPage() {
  const { effectiveChannelId, scopeInput, selectedChannel } = useConfigScope()
  const permissions = usePermissions()
  const registry = useSaveRegistry('Payment provider setup')
  const [params, setParams] = useSearchParams()
  const providers = useQuery(PaymentProvidersDocument, { variables: { channelId: effectiveChannelId } })
  const canViewConfig = effectiveChannelId
    ? permissions.canAccessChannelScope('config.view', effectiveChannelId)
    : permissions.allowedChannelScopes('config.view').allChannels
  const settings = useQuery(ConfigSectionsDocument, { variables: { scope: scopeInput }, skip: !canViewConfig })
  const [confirm, setConfirm] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const installed = (providers.data?.paymentProviders ?? []).filter((p) => p.pluginSlug || !p.isOffline)
  const selected = installed.find((p) => p.slug === params.get('provider')) ?? installed[0]

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <DetailPageHeader title="Payment providers" meta="Configure installed payment plugins. Choose which methods shoppers see under Checkout → Payment."
          actions={<Button disabled={!registry.isDirty || saving} onClick={() => setConfirm(true)}>Save</Button>} />
        <DetailPageBody>
          <fieldset disabled={saving} className="min-w-0">
            {providers.error ? <LoadError title="Could not load payment providers" message={providers.error.message} onRetry={() => void providers.refetch()} retrying={providers.loading} />
              : providers.loading && !providers.data ? <p className="p-6 text-sm text-muted-foreground">Loading payment providers…</p>
              : <>
                <DetailPageSection title="Installed providers" description="Installation adds a plugin to the backend build. Its provider and setup fields appear here after the backend restarts."
                  trailing={<Button asChild variant="outline" size="sm"><Link to="/checkout-settings?tab=payment">Checkout payment methods</Link></Button>}>
                  <DataTable<Provider> compact data={installed} getRowId={(p) => p.slug}
                    onRowClick={(p) => setParams((prev) => { const next = new URLSearchParams(prev); next.set('provider', p.slug); return next })}
                    rowIsActive={(p) => p.slug === selected?.slug}
                    emptyState="No payment gateway plugins are installed. Manual payment methods are available under Checkout → Payment."
                    columns={[
                      { accessorKey: 'displayName', header: 'Provider' },
                      { id: 'plugin', header: 'Plugin', cell: ({ row }) => <span>{row.original.pluginName || row.original.pluginSlug}{row.original.pluginVersion ? ` · ${row.original.pluginVersion}` : ''}</span> },
                      { id: 'status', header: 'Runtime status', cell: ({ row }) => <Badge variant={row.original.isAvailable ? 'success' : 'warning'}>{row.original.isAvailable ? 'Active' : 'Disabled'}</Badge> },
                    ]} />
                </DetailPageSection>
                {selected && <DetailPageSection title={`${selected.displayName} setup`} description="Credentials, test/live mode and other settings are supplied by the plugin. Active means the plugin can run; it does not verify credentials or the storefront integration.">
                  {selected.webhookPath && <div className="mb-4 grid gap-1 rounded-md border p-3 text-sm"><span className="font-medium">Webhook endpoint</span><code className="break-all">{selected.webhookPath}</code><p className="text-xs text-muted-foreground">Append this path to the public backend URL when configuring the provider's webhook.</p></div>}
                  {canViewConfig ? <ProviderConfiguration key={JSON.stringify(scopeInput)} paths={selected.configurationPaths} sections={settings.data?.configSections} />
                    : <p className="text-sm text-muted-foreground">Viewing provider setup requires configuration viewing permission. Payment availability is managed separately in Checkout.</p>}
                </DetailPageSection>}
                <DetailPageSection title="Add a payment provider" description="A gateway needs a compatible backend plugin and its storefront integration.">
                  <div className="grid gap-2 rounded-md border p-4 text-sm">
                    <p>Ask your deployment administrator to install the gateway plugin module, regenerate the application and rebuild the backend.</p>
                    <code className="break-all">./skelet cli plugin add &lt;module&gt;@&lt;version&gt;</code>
                    <p className="text-muted-foreground">Installing the backend plugin makes its setup available here. Card entry and authentication require the corresponding storefront integration.</p>
                  </div>
                </DetailPageSection>
              </>}
          </fieldset>
        </DetailPageBody>
      </DetailPageLayout>
      <SaveConfirmDialog open={confirm} scopeLabel={selectedChannel?.name ?? 'Platform defaults'} sections={registry.dirtySections} saving={saving} error={error}
        onCancel={() => setConfirm(false)} onConfirm={async () => {
          setSaving(true); setError(null)
          try { await registry.saveAll(); await providers.refetch(); setConfirm(false) }
          catch (e) { setError(e instanceof Error ? e.message : 'Could not save provider setup') }
          finally { setSaving(false) }
        }} />
    </SaveRegistryContext.Provider>
  )
}

// One page-owned draft persists while the selected provider changes; secret values remain memory-only.
function ProviderConfiguration({ paths, sections }: { paths: string[]; sections: Parameters<typeof useConfigDrafts>[0]['sections'] }) {
  const { scopeInput } = useConfigScope()
  const drafts = useConfigDrafts({ sections, scopeInput })
  return paths.length ? <div className="rounded-md border"><SettingsContent paths={paths} drafts={drafts} /></div>
    : <p className="text-sm text-muted-foreground">This plugin has not registered any setup fields.</p>
}
