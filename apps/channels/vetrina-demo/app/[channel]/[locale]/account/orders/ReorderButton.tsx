"use client";

import { Link } from "@/app/components/LocaleLink";
import { useActionState } from "react";
import { useTranslations } from "next-intl";

import { useErrorText } from "@/app/components/account/AccountUI";
import { reorderAction, type ReorderResult } from "@/app/lib/account/actions";

export function ReorderButton({ orderId }: { orderId: string }) {
  const [state, formAction, pending] = useActionState<ReorderResult | null, FormData>(
    reorderAction,
    null,
  );
  const t = useTranslations("account.orders");
  const errorText = useErrorText();

  return (
    <div>
      <form action={formAction}>
        <input type="hidden" name="orderId" value={orderId} />
        <button
          type="submit"
          disabled={pending}
          className="inline-flex h-10 items-center rounded-md border border-border bg-background px-5 text-sm font-semibold hover:bg-accent disabled:opacity-60"
        >
          {pending ? t("reordering") : t("reorder")}
        </button>
      </form>

      {state?.ok && (
        <div className="mt-2 text-sm">
          <p>
            {t("reordered", { count: state.added ?? 0 })}{" "}
            <Link href="/cart" className="font-medium underline">
              {t("viewCart")}
            </Link>
          </p>
          {state.skipped && state.skipped.length > 0 && (
            <ul className="mt-1 list-disc pl-5 text-xs text-muted-foreground">
              {state.skipped.map((s, i) => (
                <li key={i}>
                  {s.productName || s.productSku}: {s.reason}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {state && !state.ok && (
        <p role="alert" className="mt-2 text-sm text-red-700">
          {errorText(state.error ?? "reorderFailed")}
        </p>
      )}
    </div>
  );
}
