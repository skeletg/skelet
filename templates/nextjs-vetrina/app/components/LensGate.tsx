"use client";

import { LensTracker } from "@/app/components/LensTracker";
import { LensReplay } from "@/app/components/LensReplay";
import { useMarketingConsent } from "@skelet/storefront-react/consent/use-consent";
import { useCustomerIdentity } from "@skelet/storefront-next/customer/client";

// Consent and identity resolve in the browser so the layout reads no cookie - off until then, which is the right GDPR default anyway
export function LensGate({ signalsEnabled, channelId }: { signalsEnabled: boolean; channelId: string }) {
  const { identity } = useCustomerIdentity();
  const marketing = useMarketingConsent(); // unconditional - && would short-circuit the hook
  const enabled = signalsEnabled && marketing;
  const customerId = identity?.id ?? "";
  return (
    <>
      <LensTracker enabled={enabled} channelId={channelId} customerId={customerId} />
      <LensReplay enabled={enabled} channelId={channelId} customerId={customerId} />
    </>
  );
}
