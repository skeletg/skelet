"use client";

import { UserPlusIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { useAuthUI } from "./AuthUIProvider";

export function SignInButton() {
  const { open } = useAuthUI();
  const t = useTranslations("common");
  return (
    <button
      type="button"
      onClick={() => open("login")}
      aria-label={t("signIn")}
      title={t("signIn")}
      className="relative flex h-9 w-9 items-center justify-center rounded-md text-foreground transition hover:bg-muted"
    >
      <UserPlusIcon className="h-5 w-5" aria-hidden="true" />
    </button>
  );
}
