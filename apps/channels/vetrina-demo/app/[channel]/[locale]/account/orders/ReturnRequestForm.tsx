"use client";

import { useTranslations } from "next-intl";

import {
  FieldErrors,
  FormErrors,
  inputClass,
} from "@/app/components/account/AccountUI";
import { requestReturnAction } from "@/app/lib/account/actions";
import { returnRequestSchema } from "@/app/lib/account/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export type ReturnableItem = {
  orderLineId: string;
  name: string;
  sku: string;
  returnableQty: number;
};

export function ReturnRequestForm({
  orderId,
  items,
}: {
  orderId: string;
  items: ReturnableItem[];
}) {
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(
    requestReturnAction,
    returnRequestSchema,
    {
      defaultValue: {
        orderId,
        resolution: "refund",
        lines: items.map((it) => ({ orderLineId: it.orderLineId, quantity: 0 })),
      },
    },
  );
  const lineFields = fields.lines.getFieldList();
  const t = useTranslations("account.returnForm");

  if (lastResult?.rmaNumber) {
    return (
      <div className="rounded-lg border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-800">
        {t.rich("submitted", {
          rma: () => <span className="font-mono">{lastResult.rmaNumber}</span>,
        })}
      </div>
    );
  }

  return (
    <form
      id={form.id}
      onSubmit={onSubmit}
      action={formAction}
      noValidate
      className="space-y-4"
    >
      <input type="hidden" name={fields.orderId.name} value={orderId} />
      <ul className="divide-y divide-border border-y border-border">
        {items.map((it, i) => {
          // lineFields mirrors items - the default value seeds one entry per returnable line
          const qty = lineFields[i]?.getFieldset().quantity;
          const initial = qty?.initialValue;
          return (
            <li key={it.orderLineId} className="py-3">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {it.name}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t("upTo", { sku: it.sku, max: it.returnableQty })}
                  </p>
                </div>
                <input
                  type="hidden"
                  name={`${fields.lines.name}[${i}].orderLineId`}
                  value={it.orderLineId}
                />
                <input
                  key={qty?.key}
                  type="number"
                  name={`${fields.lines.name}[${i}].quantity`}
                  min={0}
                  max={it.returnableQty}
                  step={1}
                  defaultValue={typeof initial === "string" ? initial : 0}
                  aria-label={t("qtyLabel", { name: it.name })}
                  aria-invalid={qty?.errors ? true : undefined}
                  aria-describedby={qty?.errors ? qty.errorId : undefined}
                  className={`${inputClass} w-20 text-right`}
                />
              </div>
              {qty?.errors ? (
                <FieldErrors id={qty.errorId} errors={qty.errors} />
              ) : null}
            </li>
          );
        })}
      </ul>

      <fieldset className="text-sm">
        <legend className="mb-1">{t("resolution")}</legend>
        <label className="mr-4 inline-flex items-center gap-1.5">
          <input
            type="radio"
            key={fields.resolution.key}
            name={fields.resolution.name}
            value="refund"
            defaultChecked={fields.resolution.initialValue !== "exchange"}
            className="h-3.5 w-3.5"
          />
          {t("refund")}
        </label>
        <label className="inline-flex items-center gap-1.5">
          <input
            type="radio"
            key={fields.resolution.key}
            name={fields.resolution.name}
            value="exchange"
            defaultChecked={fields.resolution.initialValue === "exchange"}
            className="h-3.5 w-3.5"
          />
          {t("exchange")}
        </label>
      </fieldset>

      <label className="block text-sm">
        {t("reason")}
        <textarea
          key={fields.reason.key}
          name={fields.reason.name}
          defaultValue={fields.reason.initialValue ?? ""}
          rows={2}
          className={`${inputClass} mt-1 w-full`}
          placeholder={t("reasonPlaceholder")}
        />
      </label>

      <FormErrors errors={form.errors} />

      <button
        type="submit"
        disabled={pending}
        className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-60"
      >
        {pending ? t("submitting") : t("submit")}
      </button>
    </form>
  );
}
