import { redirect } from "next/navigation";
import { getLocale } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";

import {
  AuthCard,
  FormSuccess,
} from "@/app/components/account/AccountUI";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { safeReturnTo } from "@/app/lib/auth/return-to";
import { LoginForm } from "./LoginForm";

export const metadata = { title: "Sign in" };


export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{
    returnTo?: string | string[];
    reset?: string | string[];
    verify?: string | string[];
  }>;
}) {
  const sp = await searchParams;
  const returnTo = safeReturnTo(sp.returnTo);
  const justReset =
    (Array.isArray(sp.reset) ? sp.reset[0] : sp.reset) === "ok";
  const needsVerify =
    (Array.isArray(sp.verify) ? sp.verify[0] : sp.verify) === "required";

  const customer = await getCustomerOrNull();
  if (customer) redirect(await withLocalePrefix(returnTo ?? "/account", await getLocale()));

  return (
    <AuthCard
      title="Sign in"
      description="Welcome back. Sign in to view your orders and addresses."
      banner={
        needsVerify ? (
          <FormSuccess>
            Account created. Check your inbox and confirm your email address,
            then sign in.
          </FormSuccess>
        ) : justReset ? (
          <FormSuccess>
            Password updated. Sign in with your new password to continue.
          </FormSuccess>
        ) : undefined
      }
    >
      <LoginForm returnTo={returnTo} />
    </AuthCard>
  );
}
