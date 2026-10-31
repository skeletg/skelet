"use client";

import { useTranslations } from "next-intl";

import { Field, FormErrors, buttonPrimary } from "@/app/components/account/AccountUI";
import { useActionForm } from "@skelet/storefront-next/forms/use-action-form";
import { trackOrderAction } from "@skelet/storefront-next/order/actions";
import { trackOrderSchema } from "@skelet/storefront-next/order/schemas";

export function TrackOrderForm({ number, email }: { number?: string; email?: string }) {
  const t = useTranslations("trackOrder");
  const tf = useTranslations("forms.fields");
  const { form, fields, formAction, onSubmit, pending } = useActionForm(trackOrderAction, trackOrderSchema, {
    defaultValue: { number, email },
  });

  return (
    <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="flex flex-col gap-4">
      <Field label={t("orderNumber")} field={fields.number} required autoComplete="off" />
      <Field label={tf("email")} field={fields.email} type="email" required autoComplete="email" />
      <Field label={t("postalCode")} field={fields.postalCode} required autoComplete="postal-code" />

      <FormErrors errors={form.errors} />

      <button type="submit" disabled={pending} className={`${buttonPrimary} w-full`}>
        {pending ? t("submitting") : t("submit")}
      </button>
    </form>
  );
}
