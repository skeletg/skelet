"use client";

import { Link } from "@/app/components/LocaleLink";
import { useActionState } from "react";

import {
  Field,
  FormError,
  buttonPrimary,
  inputClass,
} from "@/app/components/account/AccountUI";
import { registerAction, type AuthResult } from "@/app/lib/auth/actions";

export function RegisterForm({
  returnTo,
  referralCode,
  onSwitchView,
}: {
  returnTo?: string | null;
  referralCode?: string | null;
  onSwitchView?: (view: "login") => void;
}) {
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(registerAction, null);

  const loginHref = returnTo
    ? `/account/login?returnTo=${encodeURIComponent(returnTo)}`
    : "/account/login";

  return (
    <form action={formAction} className="flex flex-col gap-3">
      {returnTo ? (
        <input type="hidden" name="returnTo" value={returnTo} />
      ) : null}
      {referralCode ? (
        <input type="hidden" name="referralCode" value={referralCode} />
      ) : null}
      <div className="grid grid-cols-2 gap-4">
        <Field
          label="First name"
          name="firstName"
          required
          autoComplete="given-name"
        />
        <Field
          label="Last name"
          name="lastName"
          required
          autoComplete="family-name"
        />
      </div>
      <Field label="Email" name="email" type="email" required autoComplete="email" />
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">Password</span>
        <input
          name="password"
          type="password"
          required
          autoComplete="new-password"
          minLength={8}
          className={`${inputClass} w-full`}
        />
        <span className="text-xs text-muted-foreground">
          At least 8 characters.
        </span>
      </label>
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">Confirm password</span>
        <input
          name="confirmPassword"
          type="password"
          required
          autoComplete="new-password"
          minLength={8}
          className={`${inputClass} w-full`}
        />
      </label>
      <label className="flex items-center gap-2.5 text-sm">
        <input
          type="checkbox"
          name="marketingOptIn"
          className="size-4 rounded border-border"
        />
        Send me occasional updates by email.
      </label>

      {state && !state.ok && <FormError>{state.error}</FormError>}

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? "Creating account…" : "Create account"}
      </button>

      <p className="mt-2 text-center text-sm text-muted-foreground">
        Already have one?{" "}
        {onSwitchView ? (
          <button
            type="button"
            onClick={() => onSwitchView("login")}
            className="text-foreground underline underline-offset-2"
          >
            Sign in
          </button>
        ) : (
          <Link
            href={loginHref}
            className="text-foreground underline underline-offset-2"
          >
            Sign in
          </Link>
        )}
      </p>
    </form>
  );
}
