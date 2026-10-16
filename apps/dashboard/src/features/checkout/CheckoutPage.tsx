import { useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@apollo/client/react'
import { useSearchParams } from 'react-router-dom'
import { DetailPageBody, DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CheckoutPaymentSettings } from '@/features/payments/CheckoutPaymentSettings'
import { PaymentAvailability } from '@/features/payments/PaymentAvailability'
import { SettingsContent } from '@/features/settings/SettingsContent'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { useConfigDrafts, type ConfigDrafts } from '@/features/settings/useConfigDrafts'
import { ShippingCarrierAvailability } from '@/features/shipping/ShippingCarrierAvailability'
import { DetailPageSection } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { CheckoutSettingsDocument } from './Checkout.gql'
import { DeliveryTab } from './DeliveryTab'
import { DELIVERY_PATHS, DETAILS_PATHS, TABS, isTab, type TabId } from './labels'
import { OverviewTab } from './OverviewTab'
import { PointsTab } from './PointsTab'
import type { GoTo } from './ReadinessList'
import type { Configuration } from './types'
import { useCheckoutDraft } from './useCheckoutDraft'

export function CheckoutPage() {
  const { effectiveChannelId, scopeInput } = useConfigScope()
  const { canAccessChannelScope } = usePermissions()
  const result = useQuery(CheckoutSettingsDocument, { variables: { channelId: effectiveChannelId ?? '' }, skip: !effectiveChannelId })
  const registry = useSaveRegistry('Checkout settings')
  const [, setParams] = useSearchParams()
  // Bumped after a save or adopt - the editor remounts from the refetched configuration instead of patching its draft
  const [version, setVersion] = useState(0)
  const [confirm, setConfirm] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const createdAfterSave = useRef<Map<string, string> | undefined>(undefined)

  const canManage = !!effectiveChannelId && canAccessChannelScope('checkout.manage', effectiveChannelId)

  async function reload(created?: Map<string, string>) {
    await result.refetch()
    // A new option's draft id becomes its saved id - keep it selected
    if (created?.size) {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          for (const key of ['option', 'point']) {
            const selected = prev.get(key)
            const id = selected && created.get(selected)
            if (id) next.set(key, id)
          }
          return next
        },
        { replace: true },
      )
    }
    setVersion((v) => v + 1)
  }

  let body
  if (!effectiveChannelId) body = <p className="p-6 text-sm text-muted-foreground">Select a channel to set up its checkout.</p>
  else if (!result.data && result.loading) body = <p className="p-6 text-sm text-muted-foreground">Loading checkout settings…</p>
  else if (!result.data)
    body = (
      <p role="alert" className="p-6 text-sm text-destructive">
        {result.error?.message ?? 'Checkout settings are unavailable.'}
      </p>
    )
  else {
    const configuration = result.data.checkoutConfiguration
    body = (
      <ConfigDraftsHost key={effectiveChannelId} scopeInput={scopeInput}>
        {(config) => (
          <Editor
            key={`${effectiveChannelId}:${version}`}
            channelId={effectiveChannelId}
            configuration={configuration}
            canManage={canManage}
            config={config}
            dirtySectionIds={registry.dirtySections.map((s) => s.id)}
            onReload={reload}
            onSaved={async (created) => { createdAfterSave.current = created }}
          />
        )}
      </ConfigDraftsHost>
    )
  }

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <DetailPageHeader
          title="Checkout"
          meta="Everything a shopper goes through between the cart and placing the order: their details, how the order reaches them, and how they pay"
          actions={
            <Button disabled={!registry.isDirty || saving} onClick={() => setConfirm(true)}>
              Save
            </Button>
          }
        />
        <DetailPageBody><fieldset disabled={saving} className="min-w-0">{body}</fieldset></DetailPageBody>
      </DetailPageLayout>
      <SaveConfirmDialog
        open={confirm}
        scopeLabel="Selected channel"
        sections={registry.dirtySections}
        saving={saving}
        error={error}
        onCancel={() => setConfirm(false)}
        onConfirm={async () => {
          setSaving(true)
          setError(null)
          try {
            await registry.saveAll()
            await reload(createdAfterSave.current)
            createdAfterSave.current = undefined
            setConfirm(false)
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Could not save checkout settings')
          } finally {
            setSaving(false)
          }
        }}
      />
    </SaveRegistryContext.Provider>
  )
}

// The checkout settings stage like Settings' own - the host sits outside the Editor, which remounts after every delivery save,
// so a remount can't drop staged settings mid-save
function ConfigDraftsHost({
  scopeInput,
  children,
}: {
  scopeInput: ReturnType<typeof useConfigScope>['scopeInput']
  children: (config: ConfigDrafts) => ReactNode
}) {
  const { data } = useQuery(ConfigSectionsDocument, { variables: { scope: scopeInput } })
  const config = useConfigDrafts({ sections: data?.configSections, scopeInput })
  return children(config)
}

function Editor({
  channelId,
  configuration,
  canManage,
  config,
  onReload,
  dirtySectionIds,
  onSaved,
}: {
  channelId: string
  configuration: Configuration
  canManage: boolean
  config: ConfigDrafts
  onReload: (created?: Map<string, string>) => Promise<void>
  dirtySectionIds: string[]
  onSaved: (created: Map<string, string>) => Promise<void>
}) {
  const [params, setParams] = useSearchParams()
  const draft = useCheckoutDraft({ channelId, configuration, onSaved })
  const requested = params.get('tab')
  const tab: TabId = isTab(requested) ? requested : 'overview'
  const [visited, setVisited] = useState<Set<TabId>>(() => new Set([tab]))
  // Built-in options aren't stored, so nothing can be edited until they're saved as the channel's own
  const locked = !canManage || !draft.adopted
  const staged = (paths: readonly string[]) => paths.some((p) => p in config.drafts)
  const dirtyTabs = new Set<TabId>(draft.dirtyTabs)
  if (staged(DETAILS_PATHS)) dirtyTabs.add('details')
  if (staged(DELIVERY_PATHS) || dirtySectionIds.includes('carrier-availability')) dirtyTabs.add('delivery')
  if ((dirtySectionIds.includes('payment-availability') || dirtySectionIds.includes('payment-presentation')) || Object.keys(config.drafts).some((p) => p.startsWith('payment/'))) dirtyTabs.add('payment')

  const goTo: GoTo = (next, extra) => {
    setVisited((prev) => new Set([...prev, tab, next]))
    setParams((prev) => {
      const p = new URLSearchParams(prev)
      p.set('tab', next)
      p.delete('option')
      p.delete('point')
      if (extra?.option) p.set('option', extra.option)
      return p
    })
  }
  const select = (key: 'option' | 'point') => (id: string | null) =>
    setParams((prev) => {
      const p = new URLSearchParams(prev)
      if (id) p.set(key, id)
      else p.delete(key)
      return p
    })

  return (
    <Tabs value={tab} onValueChange={(v) => goTo(v as TabId)}>
      <div className="overflow-x-auto px-6 pt-6">
        <TabsList>
          {TABS.map((t) => (
            <TabsTrigger key={t.id} value={t.id}>
              {t.label}
              {dirtyTabs.has(t.id) && <span className="ml-1.5 size-1.5 rounded-full bg-warning" aria-label="unsaved changes" />}
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      {!draft.adopted && (tab === 'delivery' || tab === 'points') && (
        <div className="mx-6 mt-4 flex flex-wrap items-center gap-3 rounded-md border border-info/40 bg-info-soft p-3 text-sm">
          <span className="flex-1">This channel uses the built-in checkout. Start customising to make changes here.</span>
          <Button size="sm" variant="outline" onClick={() => goTo('overview')}>
            Go to Overview
          </Button>
        </div>
      )}
      {(visited.has('overview') || tab === 'overview') && (
        <TabsContent forceMount hidden={tab !== 'overview'} value="overview" className="mt-0 data-[state=inactive]:hidden">
          <OverviewTab
            channelId={channelId}
            configuration={configuration}
            draft={draft}
            canManage={canManage}
            onGo={goTo}
            onAdopted={() => onReload()}
            hasUnsavedChanges={dirtySectionIds.length > 0}
          />
        </TabsContent>
      )}
      {(visited.has('delivery') || tab === 'delivery') && (
        <TabsContent forceMount hidden={tab !== 'delivery'} value="delivery" className="mt-0 data-[state=inactive]:hidden">
          <DeliveryTab
            configuration={configuration}
            draft={draft}
            selectedId={params.get('option')}
            disabled={locked}
            onSelect={select('option')}
            onGo={goTo}
          />
          <DetailPageSection title="Pickup orders" description="Who may collect a pickup order, and what the shopper enters for one.">
            <div className="rounded-md border">
              <SettingsContent activeSectionSlug="sales" activeGroupSlug="checkout" paths={DELIVERY_PATHS} drafts={config} />
            </div>
          </DetailPageSection>
          <ShippingCarrierAvailability disabled={!canManage} />
        </TabsContent>
      )}
      {(visited.has('details') || tab === 'details') && (
        <TabsContent forceMount hidden={tab !== 'details'} value="details" className="mt-0 data-[state=inactive]:hidden">
          <DetailPageSection
            title="Shopper details"
            description="What the details step asks for, who can check out, and the rules an order must pass before it can be placed."
          >
            <div className="rounded-md border">
              <SettingsContent activeSectionSlug="sales" activeGroupSlug="checkout" paths={DETAILS_PATHS} drafts={config} />
            </div>
          </DetailPageSection>
        </TabsContent>
      )}
      {(visited.has('payment') || tab === 'payment') && (
        <TabsContent forceMount hidden={tab !== 'payment'} value="payment" className="mt-0 data-[state=inactive]:hidden">
          <PaymentAvailability />
          <CheckoutPaymentSettings config={config} />
        </TabsContent>
      )}
      {(visited.has('points') || tab === 'points') && (
        <TabsContent forceMount hidden={tab !== 'points'} value="points" className="mt-0 data-[state=inactive]:hidden">
          <PointsTab draft={draft} selectedKey={params.get('point')} disabled={locked} onSelect={select('point')} />
        </TabsContent>
      )}
    </Tabs>
  )
}
