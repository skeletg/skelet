"use client";

import { Link } from "@/app/components/LocaleLink";
import { UserIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { useCustomerIdentity } from "@skelet/storefront-next/customer/client";
import { SignInButton } from "@/app/components/SignInButton";

// Same 9x9 footprint in every state, so resolving identity after hydration never shifts the header
export function AccountSlot() {
  const { identity, resolved } = useCustomerIdentity();
  const t = useTranslations("auth");
  const th = useTranslations("header");
  if (!resolved) return <span aria-hidden="true" className="h-9 w-9 rounded-md bg-muted animate-pulse" />;
  if (!identity) return <SignInButton />;
  return (
    <Link
      href="/account"
      aria-label={t("accountLabel", { name: identity.firstName })}
      title={th("hi", { name: identity.firstName })}
      // lens-block: the label and title carry the customer's name, and rrweb masks text, not attributes
      className="lens-block relative flex h-9 w-9 items-center justify-center rounded-md text-foreground transition hover:bg-muted"
    >
      <UserIcon className="h-5 w-5" aria-hidden="true" />
      <span
        aria-hidden="true"
        className="absolute right-1 top-1 h-2 w-2 rounded-full bg-emerald-500 ring-2 ring-background"
      />
    </Link>
  );
}
