import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { SettingsContent } from '@/features/settings/SettingsContent'
import type { ConfigDrafts } from '@/features/settings/useConfigDrafts'
import { useConfigScope } from '@/lib/config-scope'
import { PaymentMethodPresentation } from './PaymentMethodPresentation'
import { PaymentMethodsDocument } from './Payments.gql'

export function CheckoutPaymentSettings({ config }: { config: ConfigDrafts }) {
  const { effectiveChannelId } = useConfigScope()
  const { data } = useQuery(PaymentMethodsDocument, { variables: { channelId: effectiveChannelId ?? '' }, skip: !effectiveChannelId })
  const instructionPaths = (data?.paymentMethods ?? []).filter((m) => m.isOffline).map((m) => `payment/${m.providerSlug}/instructions`)
  return <>
    <PaymentMethodPresentation />
    {instructionPaths.length > 0 && <DetailPageSection title="Manual payment instructions" description="What shoppers see for cash, bank transfers and other manual payments."><div className="rounded-md border"><SettingsContent paths={instructionPaths} drafts={config} /></div></DetailPageSection>}
    <DetailPageSection title="Gateway setup" description="Credentials, test/live mode and plugin setup are managed in Payment providers."
      trailing={<Button asChild variant="outline" size="sm"><Link to="/payment-providers">Payment providers</Link></Button>}>
      <p className="text-sm text-muted-foreground">Installed providers appear automatically. Card payments also need the gateway's storefront integration.</p>
    </DetailPageSection>
  </>
}
