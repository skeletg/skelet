"use client";

import { useActionState } from "react";

import {
  Field,
  FormError,
  FormSuccess,
  buttonPrimary,
  inputClass,
} from "@/app/components/account/AccountUI";
import { updateProfileAction, type AuthResult } from "@/app/lib/auth/actions";

type Props = {
  firstName: string;
  lastName: string;
  phone: string;
  email: string;
};

export function ProfileForm({ firstName, lastName, phone, email }: Props) {
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(updateProfileAction, null);

  return (
    <form action={formAction} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">Email</span>
        <input
          value={email}
          readOnly
          disabled
          className={`${inputClass} w-full bg-muted text-muted-foreground`}
        />
        <span className="text-xs text-muted-foreground">
          Email is permanent on this account. Contact support to change it.
        </span>
      </label>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label="First name"
          name="firstName"
          required
          defaultValue={firstName}
          autoComplete="given-name"
        />
        <Field
          label="Last name"
          name="lastName"
          required
          defaultValue={lastName}
          autoComplete="family-name"
        />
      </div>
      <Field
        label="Phone"
        name="phone"
        type="tel"
        defaultValue={phone}
        autoComplete="tel"
      />

      {state && !state.ok && <FormError>{state.error}</FormError>}
      {state?.ok && <FormSuccess>Profile saved.</FormSuccess>}

      <div className="mt-1">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? "Saving…" : "Save changes"}
        </button>
      </div>
    </form>
  );
}
