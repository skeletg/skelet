import { redirect } from "next/navigation";
import { getLocale } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";

import { AuthCard } from "@/app/components/account/AccountUI";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { safeReturnTo } from "@/app/lib/auth/return-to";
import { RegisterForm } from "./RegisterForm";

export const metadata = { title: "Create account" };


export default async function RegisterPage({
  searchParams,
}: {
  searchParams: Promise<{ returnTo?: string | string[]; ref?: string | string[] }>;
}) {
  const sp = await searchParams;
  const returnTo = safeReturnTo(sp.returnTo);
  const refRaw = Array.isArray(sp.ref) ? sp.ref[0] : sp.ref;
  const referralCode = refRaw?.trim() || null;

  const customer = await getCustomerOrNull();
  if (customer) redirect(await withLocalePrefix(returnTo ?? "/account", await getLocale()));

  return (
    <AuthCard
      title="Create account"
      description="Saves your addresses and keeps every order in one place."
    >
      <RegisterForm returnTo={returnTo} referralCode={referralCode} />
    </AuthCard>
  );
}
