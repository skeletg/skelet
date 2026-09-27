"use client";

import { useMemo } from "react";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { resetPasswordAction } from "@/app/lib/auth/actions";
import { resetSchema } from "@/app/lib/auth/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export function ResetForm({ token }: { token: string }) {
  // Bound, not a hidden field - a hidden input is captured by rrweb (maskAllInputs has
  // no entry for type=hidden), which would ship a live reset token into session replay
  const boundAction = useMemo(
    () => resetPasswordAction.bind(null, token),
    [token],
  );
  const { form, fields, formAction, pending } = useActionForm(
    boundAction,
    resetSchema,
  );

  return (
    <form
      id={form.id}
      onSubmit={form.onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-4"
    >
      <Field
        label="New password"
        field={fields.newPassword}
        type="password"
        required
        minLength={8}
        autoComplete="new-password"
      />
      <Field
        label="Confirm password"
        field={fields.confirmPassword}
        type="password"
        required
        minLength={8}
        autoComplete="new-password"
      />

      <FormErrors errors={form.errors} />

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
