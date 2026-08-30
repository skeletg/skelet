"use client";

import { Link } from "@/app/components/LocaleLink";
import { useActionState } from "react";

import {
  Field,
  FormError,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import {
  requestPasswordResetAction,
  type AuthResult,
} from "@/app/lib/auth/actions";

export function ForgotForm({
  onSwitchView,
}: {
  onSwitchView?: (view: "login") => void;
} = {}) {
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(requestPasswordResetAction, null);

  // After the mutation succeeds we render a static success message in place of the form
  // - even if the email doesn't exist, the backend returns ok and we stay quiet about
  // it (anti-enumeration)
  if (state?.ok) {
    return (
      <div className="rounded-lg border border-border bg-muted p-4 text-sm">
        <p>
          If an account exists for that email, you&rsquo;ll receive a
          reset link shortly. The link expires after 30 minutes.
        </p>
        <p className="mt-3 text-muted-foreground">
          {onSwitchView ? (
            <button
              type="button"
              onClick={() => onSwitchView("login")}
              className="text-foreground underline underline-offset-2"
            >
              Back to sign in
            </button>
          ) : (
            <Link
              href="/account/login"
              className="text-foreground underline underline-offset-2"
            >
              Back to sign in
            </Link>
          )}
        </p>
      </div>
    );
  }

  return (
    <form action={formAction} className="flex flex-col gap-4">
      <Field label="Email" name="email" type="email" required autoComplete="email" />

      {state && !state.ok && <FormError>{state.error}</FormError>}

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? "Sending…" : "Send reset link"}
      </button>

      <p className="mt-2 text-center text-sm text-muted-foreground">
        Remembered it?{" "}
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
            href="/account/login"
            className="text-foreground underline underline-offset-2"
          >
            Sign in
          </Link>
        )}
      </p>
    </form>
  );
}
