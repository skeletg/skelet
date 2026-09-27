"use client";

import {
  Field,
  FormErrors,
  FormSuccess,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { changePasswordAction } from "@/app/lib/auth/actions";
import { changePasswordSchema } from "@/app/lib/auth/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export function PasswordForm() {
  const { form, fields, formAction, pending, lastResult } = useActionForm(
    changePasswordAction,
    changePasswordSchema,
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
        label="Current password"
        field={fields.currentPassword}
        type="password"
        required
        autoComplete="current-password"
      />
      <Field
        label="New password"
        field={fields.newPassword}
        type="password"
        required
        autoComplete="new-password"
        minLength={8}
        hint="At least 8 characters."
      />
      <Field
        label="Confirm new password"
        field={fields.confirmPassword}
        type="password"
        required
        autoComplete="new-password"
        minLength={8}
      />

      <FormErrors errors={form.errors} />
      {lastResult?.status === "success" && (
        <FormSuccess>Password updated.</FormSuccess>
      )}

      <div className="mt-1">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? "Saving…" : "Update password"}
        </button>
      </div>
    </form>
  );
}
