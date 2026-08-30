"use client";

import { UserPlusIcon } from "lucide-react";

import { useAuthUI } from "./AuthUIProvider";

export function SignInButton() {
  const { open } = useAuthUI();
  return (
    <button
      type="button"
      onClick={() => open("login")}
      aria-label="Sign in"
      title="Sign in"
      className="relative flex h-9 w-9 items-center justify-center rounded-md text-foreground transition hover:bg-muted"
    >
      <UserPlusIcon className="h-5 w-5" aria-hidden="true" />
    </button>
  );
}
