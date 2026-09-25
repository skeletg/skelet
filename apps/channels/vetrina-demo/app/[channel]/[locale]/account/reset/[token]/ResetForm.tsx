"use client";

import { useActionState, useMemo } from "react";

import {
  FormError,
  buttonPrimary,
  inputClass,
} from "@/app/components/account/AccountUI";
import {
  resetPasswordAction,
  type AuthResult,
} from "@/app/lib/auth/actions";

export function ResetForm({ token }: { token: string }) {
  // Bound, not a hidden field - a hidden input is captured by rrweb (maskAllInputs has
  // no entry for type=hidden), which would ship a live reset token into session replay
  const boundAction = useMemo(
    () => resetPasswordAction.bind(null, token),
    [token],
  );
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(boundAction, null);

  return (
    <form action={formAction} className="flex flex-col gap-4">
      <PasswordField label="New password" name="newPassword" />
      <PasswordField label="Confirm password" name="confirmPassword" />

      {state && !state.ok && <FormError>{state.error}</FormError>}

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? "Saving…" : "Save new password"}
      </button>
    </form>
  );
}

function PasswordField({ label, name }: { label: string; name: string }) {
  return (
    <label className="flex flex-col gap-1.5 text-sm">
      <span className="font-medium">{label}</span>
      <input
        name={name}
        type="password"
        required
        minLength={8}
        autoComplete="new-password"
        className={`${inputClass} w-full`}
      />
    </label>
  );
}
