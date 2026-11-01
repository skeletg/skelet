"use client";

import { useEffect, useRef } from "react";
import { useTranslations } from "next-intl";

import { FieldErrors, FormErrors } from "@/app/components/account/AccountUI";
import { notifyBackInStockAction } from "@skelet/storefront-next/beacons/actions";
import { backInStockSchema } from "@skelet/storefront-core/beacons/schemas";
import { useCustomerIdentity } from "@skelet/storefront-next/customer/client";
import { useActionForm } from "@skelet/storefront-react/forms/use-action-form";

export function BackInStockForm({ productId, defaultEmail = "" }: { productId: string; defaultEmail?: string }) {
  const t = useTranslations("product.backInStock");
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(
    notifyBackInStockAction,
    backInStockSchema,
    { defaultValue: { productId, email: defaultEmail } },
  );
  // The page is cached HTML so the server render has no session - identity lands after hydration and fills the email,
  // unless the shopper already typed one of their own
  const identityEmail = useCustomerIdentity().identity?.email;
  const emailBox = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (identityEmail && !emailBox.current?.value) form.update({ name: fields.email.name, value: identityEmail });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [identityEmail]);

  if (lastResult?.status === "success") {
    return (
      <div role="status" className="mt-6 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-200">
        {t("done")}
      </div>
    );
  }

  return (
    <form id={form.id} action={formAction} onSubmit={onSubmit} noValidate className="mt-6 rounded-xl border border-border p-4">
      <input type="hidden" name={fields.productId.name} value={productId} />
      <label htmlFor={fields.email.id} className="text-sm font-medium">{t("title")}</label>
      <div className="mt-3 flex gap-2">
        <input
          ref={emailBox}
          id={fields.email.id}
          key={fields.email.key}
          name={fields.email.name}
          type="email"
          required
          autoComplete="email"
          defaultValue={String(fields.email.initialValue ?? "")}
          placeholder={t("placeholder")}
          aria-invalid={fields.email.errors ? true : undefined}
          aria-describedby={fields.email.errors ? fields.email.errorId : undefined}
          className="min-w-0 flex-1 rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none focus:border-ring"
        />
        <button
          type="submit"
          disabled={pending}
          className="shrink-0 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-60"
        >
          {pending ? "…" : t("submit")}
        </button>
      </div>
      {fields.email.errors ? <FieldErrors id={fields.email.errorId} errors={fields.email.errors} /> : null}
      <div className="mt-2"><FormErrors errors={form.errors} /></div>
    </form>
  );
}
