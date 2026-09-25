"use client";

import { Link } from "@/app/components/LocaleLink";
import { useActionState } from "react";

import {
  Field,
  FormError,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { loginAction, type AuthResult } from "@/app/lib/auth/actions";
export function LoginForm({
  returnTo,
  onSwitchView,
}: {
  returnTo?: string | null;
  onSwitchView?: (view: "register" | "forgot") => void;
}) {
  // useActionState is the React 19 / Next 16 form-state hook
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(loginAction, null);

  const registerHref = returnTo
    ? `/account/register?returnTo=${encodeURIComponent(returnTo)}`
    : "/account/register";

  return (
    <form action={formAction} className="flex flex-col gap-4">
      {returnTo ? (
        <input type="hidden" name="returnTo" value={returnTo} />
      ) : null}
      <Field label="Email" name="email" type="email" required autoComplete="email" />
      <Field
        label="Password"
        name="password"
        type="password"
        required
        autoComplete="current-password"
      />

      {state && !state.ok && <FormError>{state.error}</FormError>}

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? "Signing in…" : "Sign in"}
      </button>

      <p className="mt-1 text-center text-sm">
        {onSwitchView ? (
          <button
            type="button"
            onClick={() => onSwitchView("forgot")}
            className="text-muted-foreground hover:text-foreground hover:underline"
          >
            Forgot your password?
          </button>
        ) : (
          <Link
            href="/account/forgot"
            className="text-muted-foreground hover:text-foreground hover:underline"
          >
            Forgot your password?
          </Link>
        )}
      </p>

      <p className="mt-2 text-center text-sm text-muted-foreground">
        New here?{" "}
        {onSwitchView ? (
          <button
            type="button"
            onClick={() => onSwitchView("register")}
            className="text-foreground underline underline-offset-2"
          >
            Create an account
          </button>
        ) : (
          <Link
            href={registerHref}
            className="text-foreground underline underline-offset-2"
          >
            Create an account
          </Link>
        )}
      </p>
    </form>
  );
}
