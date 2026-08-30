"use client";

import { useActionState } from "react";

import {
  FormError,
  FormSuccess,
  buttonPrimary,
  inputClass,
} from "@/app/components/account/AccountUI";
import { changePasswordAction, type AuthResult } from "@/app/lib/auth/actions";

export function PasswordForm() {
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(changePasswordAction, null);

  return (
    <form action={formAction} className="flex flex-col gap-4">
      {/* Field can't carry minLength, and password inputs want it. */}
      <PasswordField
        label="Current password"
        name="currentPassword"
        autoComplete="current-password"
      />
      <PasswordField
        label="New password"
        name="newPassword"
        autoComplete="new-password"
        minLength={8}
        hint="At least 8 characters."
      />
      <PasswordField
        label="Confirm new password"
        name="confirmPassword"
        autoComplete="new-password"
        minLength={8}
      />

      {state && !state.ok && <FormError>{state.error}</FormError>}
      {state?.ok && <FormSuccess>Password updated.</FormSuccess>}

      <div className="mt-1">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? "Saving…" : "Update password"}
        </button>
      </div>
    </form>
  );
}

function PasswordField({
  label,
  name,
  autoComplete,
  minLength,
  hint,
}: {
  label: string;
  name: string;
  autoComplete: string;
  minLength?: number;
  hint?: string;
}) {
  return (
    <label className="flex flex-col gap-1.5 text-sm">
      <span className="font-medium">{label}</span>
      <input
        name={name}
        type="password"
        required
        minLength={minLength}
        autoComplete={autoComplete}
        className={`${inputClass} w-full`}
      />
      {hint ? (
        <span className="text-xs text-muted-foreground">{hint}</span>
      ) : null}
    </label>
  );
}
