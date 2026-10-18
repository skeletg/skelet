"use client";

import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  FormSuccess,
  buttonPrimary,
  inputClass,
} from "@/app/components/account/AccountUI";
import { updateProfileAction } from "@/app/lib/auth/actions";
import { profileSchema } from "@/app/lib/auth/schemas";
import type { PhoneSettings } from "@/app/lib/countries/read";
import { phoneFieldFor } from "@/app/lib/forms/phone";
import { useActionForm } from "@/app/lib/forms/use-action-form";

type Props = {
  firstName: string;
  lastName: string;
  phone: string;
  // The store's phone policy - a profile phone is read in its default country, or the one it's locked to
  phoneSettings: PhoneSettings;
  email: string;
};

export function ProfileForm({ firstName, lastName, phone, phoneSettings, email }: Props) {
  const t = useTranslations("account.profile");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(
    updateProfileAction,
    profileSchema,
    { defaultValue: { firstName, lastName, phone } },
  );

  return (
    <form
      id={form.id}
      onSubmit={onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-4"
    >
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">{tf("email")}</span>
        <input
          value={email}
          readOnly
          disabled
          className={`${inputClass} w-full bg-muted text-muted-foreground`}
        />
        <span className="text-xs text-muted-foreground">
          {t("emailHint")}
        </span>
      </label>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label={tf("firstName")}
          field={fields.firstName}
          required
          autoComplete="given-name"
        />
        <Field
          label={tf("lastName")}
          field={fields.lastName}
          required
          autoComplete="family-name"
        />
      </div>
      <Field
        label={tf("phone")}
        field={fields.phone}
        type="tel"
        phone={phoneFieldFor(phoneSettings, "")}
        autoComplete="tel"
      />

      <FormErrors errors={form.errors} />
      {lastResult?.status === "success" && (
        <FormSuccess>{t("saved")}</FormSuccess>
      )}

      <div className="mt-1">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? tc("saving") : tc("saveChanges")}
        </button>
      </div>
    </form>
  );
}
