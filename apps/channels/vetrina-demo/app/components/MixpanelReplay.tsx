"use client";

import { useEffect } from "react";

import { syncMixpanel, type MixpanelReplayConfig } from "@/app/lib/analytics/mixpanel";
import { parseConsent } from "@/app/lib/consent/categories";
import { useConsentChoice } from "@/app/lib/consent/use-consent";
import { useCustomerIdentity } from "@/app/lib/customer/client";

// Renders nothing - starts Mixpanel's session replay once the visitor allows analytics, and stops it on a withdrawal.
// Without a project token it isn't mounted at all
export function MixpanelReplay(cfg: MixpanelReplayConfig) {
  const choice = useConsentChoice();
  const { identity, resolved } = useCustomerIdentity();
  const { token, region, percent } = cfg;
  useEffect(() => {
    if (!resolved) return; // wait to know who is signed in, so a recording isn't started under the wrong person
    const analytics = parseConsent(choice)?.analytics ?? false;
    void syncMixpanel({ token, region, percent }, analytics, identity?.id ?? null, window.localStorage);
  }, [choice, identity?.id, resolved, token, region, percent]);
  return null;
}
