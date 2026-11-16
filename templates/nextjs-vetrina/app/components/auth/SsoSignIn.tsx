import { getLocale, getTranslations } from "next-intl/server";

import { buttonSecondary } from "@/app/components/account/AccountUI";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

// A plain link rather than a form or next/link: it works before the page's scripts load, a store's form-action rule
// doesn't govern where it ends up, and nothing prefetches it into starting a sign-in
export async function SsoSignIn({ label, returnTo }: { label: string; returnTo?: string | null }) {
  const [t, locale] = await Promise.all([getTranslations("auth.login"), getLocale()]);
  const query = returnTo ? `?${new URLSearchParams({ returnTo })}` : "";
  const href = await withLocalePrefix(`/account/sso/start${query}`, locale);
  return (
    <div className="mt-6 grid gap-4">
      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        {t("or")}
        <span className="h-px flex-1 bg-border" />
      </div>
      <a href={href} className={`${buttonSecondary} text-center`}>
        {label}
      </a>
    </div>
  );
}
