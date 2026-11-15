import { getTranslations } from "next-intl/server";

import { buttonSecondary } from "@/app/components/account/AccountUI";
import { ssoStartAction } from "@skelet/storefront-next/auth/sso-actions";

// A plain form, so the provider's button works before the page's scripts load
export async function SsoSignIn({ label, returnTo }: { label: string; returnTo?: string | null }) {
  const t = await getTranslations("auth.login");
  return (
    <form action={ssoStartAction} className="mt-6 grid gap-4">
      <div className="flex items-center gap-3 text-xs text-neutral-400">
        <span className="h-px flex-1 bg-neutral-200" />
        {t("or")}
        <span className="h-px flex-1 bg-neutral-200" />
      </div>
      {returnTo && <input type="hidden" name="returnTo" value={returnTo} />}
      <button type="submit" className={buttonSecondary}>
        {label}
      </button>
    </form>
  );
}
