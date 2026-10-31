"use client";

// Persistent strip while a rep shops as a customer - the marker cookie is set by /masquerade and expires with the
// session token, so the banner disappears when the masquerade does
//
// Client-side, not server: most of the site renders through the force-static catch-all, where cookies() is empty, so
// a server read made this vanish on exactly the pages a rep spends their time on while the session stayed masqueraded

import { useSyncExternalStore } from "react";
import { useTranslations } from "next-intl";

import { MASQUERADE_COOKIE } from "@skelet/storefront-next/auth/cookies";
import { useCustomerIdentity } from "@skelet/storefront-next/customer/client";

const noopSubscribe = () => () => {}; // the cookie never pushes - same shape as useMarketingConsent

function readMarker(): boolean {
  if (typeof document === "undefined") return false;
  return document.cookie.split("; ").some((row) => row === `${MASQUERADE_COOKIE}=1`);
}

export function MasqueradeBanner() {
  // false while server-rendering: a wrongly-shown "you are impersonating someone" banner is worse
  // than one that appears a beat late
  const masquerading = useSyncExternalStore(noopSubscribe, readMarker, () => false);
  const { identity } = useCustomerIdentity();
  const t = useTranslations("masquerade");
  if (!masquerading || !identity) return null;
  return (
    <div className="lens-block bg-amber-500 px-4 py-1.5 text-center text-sm font-medium text-black">
      {t("banner", {
        name: `${identity.firstName} ${identity.lastName}`,
        email: identity.email,
      })}
    </div>
  );
}
