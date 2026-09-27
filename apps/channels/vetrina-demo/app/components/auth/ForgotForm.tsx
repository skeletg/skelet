"use client";

import { Link } from "@/app/components/LocaleLink";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { requestPasswordResetAction } from "@/app/lib/auth/actions";
import { forgotSchema } from "@/app/lib/auth/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export function ForgotForm({
  onSwitchView,
}: {
  onSwitchView?: (view: "login") => void;
} = {}) {
  const { form, fields, formAction, pending, lastResult } = useActionForm(
    requestPasswordResetAction,
    forgotSchema,
  );

  // After the mutation succeeds we render a static success message in place of the form
  // - even if the email doesn't exist, the backend returns ok and we stay quiet about
  // it (anti-enumeration)
  if (lastResult?.status === "success") {
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
    <form
      id={form.id}
      onSubmit={form.onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-4"
    >
      <Field
        label="Email"
        field={fields.email}
        type="email"
        required
        autoComplete="email"
      />

      <FormErrors errors={form.errors} />

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
