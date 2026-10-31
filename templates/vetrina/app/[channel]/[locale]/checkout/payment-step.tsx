"use client";

import { CheckoutCartFields } from "@/app/components/offer/CheckoutCartContext";

import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import { FieldErrors, FormErrors, inputClass } from "@/app/components/account/AccountUI";
import { Link } from "@/app/components/LocaleLink";
import {
  applyGiftCardAction,
  placeOrderAction,
  removeGiftCardAction,
  setStoreCreditAction,
} from "@skelet/storefront-next/checkout/actions";
import type { CheckoutState, PaymentMethodOption } from "@skelet/storefront-next/checkout/read";
import { giftCardSchema, paymentStepSchema } from "@skelet/storefront-next/checkout/schemas";
import type { CheckoutSettings } from "@skelet/storefront-next/checkout/settings";
import { formatPrice } from "@skelet/storefront-next/format";
import { useActionForm } from "@skelet/storefront-next/forms/use-action-form";
import type { SavedPaymentMethod } from "@skelet/storefront-next/payment-methods/actions";

import { ChoiceCard } from "./fields";

type Currency = string | { code: string; symbol?: string; symbolPosition?: string };

export function PaymentStep({
  methods,
  checkout,
  note,
  savedCards,
  storeCreditBalance,
  signedIn,
  consent,
  locale,
  currency,
}: {
  methods: PaymentMethodOption[];
  checkout: CheckoutState;
  note: string;
  savedCards: SavedPaymentMethod[];
  storeCreditBalance: number;
  signedIn: boolean;
  consent: CheckoutSettings["termsConsent"];
  locale: string;
  currency: Currency;
}) {
  const t = useTranslations("checkout.payment");
  const tenders = checkout.totals.giftCardCents + checkout.totals.storeCreditCents;
  return (
    <div className="grid gap-6">
      {/* The step reads like a receipt - what's owed first, then how to settle it */}
      <p className="flex items-baseline justify-between gap-3 border-b border-border pb-3 text-sm">
        <span className="text-muted-foreground">{tenders > 0 ? t("toPay") : t("orderTotal")}</span>
        <span className="text-lg font-semibold tabular-nums">
          {formatPrice(checkout.totals.amountDueCents, currency)}
        </span>
      </p>
      <GiftCards giftCards={checkout.giftCards} currency={currency} />
      {signedIn && storeCreditBalance > 0 ? (
        <StoreCredit using={checkout.useStoreCredit} balance={storeCreditBalance} currency={currency} />
      ) : null}
      <PlaceOrderForm
        methods={methods}
        checkout={checkout}
        note={note}
        savedCards={savedCards}
        consent={consent}
        locale={locale}
        currency={currency}
      />
    </div>
  );
}

function PlaceOrderForm({
  methods,
  checkout,
  note,
  savedCards,
  consent,
  locale,
  currency,
}: {
  methods: PaymentMethodOption[];
  checkout: CheckoutState;
  note: string;
  savedCards: SavedPaymentMethod[];
  consent: CheckoutSettings["termsConsent"];
  locale: string;
  currency: Currency;
}) {
  const t = useTranslations("checkout.payment");
  // Gift cards and store credit covering the total leave nothing for a method to charge
  const covered = checkout.totals.amountDueCents <= 0;
  const provider = covered ? "" : (checkout.paymentProviderSlug ?? methods[0]?.providerSlug ?? "");
  const schema = useMemo(() => paymentStepSchema({ requireMethod: !covered, consent }), [covered, consent]);
  const { form, fields, formAction, onSubmit, pending } = useActionForm(placeOrderAction, schema, {
    defaultValue: { providerSlug: provider, savedPaymentMethodId: "", note },
  });
  const picked = typeof fields.providerSlug.value === "string" ? fields.providerSlug.value : provider;

  if (!covered && methods.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  }

  // What follows the chosen method, rendered once inside it: saved cards, the note, consent and the Pay button
  const finish = (
    <div className="grid gap-4">
      <SavedCards cards={savedCards.filter((c) => c.providerSlug === picked)} field={fields.savedPaymentMethodId} />
      <details className="text-sm" open={note !== ""}>
        <summary className="cursor-pointer text-xs underline underline-offset-2">{t("addNote")}</summary>
        <label className="mt-2 flex flex-col gap-1.5">
          <span className="sr-only">{t("note")}</span>
          <textarea
            key={fields.note.key}
            name={fields.note.name}
            defaultValue={typeof fields.note.initialValue === "string" ? fields.note.initialValue : ""}
            rows={2}
            maxLength={2000}
            placeholder={t("notePlaceholder")}
            className={`${inputClass} w-full`}
          />
        </label>
      </details>
      <Consent consent={consent} field={fields.tos} />
      <button
        type="submit"
        disabled={pending}
        className="inline-flex h-12 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
      >
        {pending
          ? t("placing")
          : covered
            ? t("placeCovered")
            : t("pay", { amount: formatPrice(checkout.totals.amountDueCents, currency) })}
      </button>
    </div>
  );

  return (
    <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-4">
        <CheckoutCartFields />
      <input type="hidden" name="locale" value={locale} />
      <FormErrors errors={form.errors} />

      {covered ? (
        <>
          <p className="text-sm text-muted-foreground">{t("covered")}</p>
          {finish}
        </>
      ) : (
        <fieldset className="grid gap-2">
          <legend className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("method")}
          </legend>
          <div className="grid overflow-hidden rounded-lg border border-border">
          {methods.map((m) => {
            const open = picked === m.providerSlug;
            return (
              <div
                key={m.providerSlug}
                className={`border-b border-border last:border-b-0 ${open ? "bg-muted/40" : ""}`}
              >
                <label className="flex cursor-pointer items-center gap-3 px-4 py-3 text-sm font-medium">
                  <input
                    type="radio"
                    name={fields.providerSlug.name}
                    value={m.providerSlug}
                    defaultChecked={provider === m.providerSlug}
                    aria-controls={open ? `${form.id}-finish` : undefined}
                    className="size-4 accent-primary"
                  />
                  {m.displayName}
                </label>
                {open ? (
                  <div id={`${form.id}-finish`} className="grid gap-4 px-4 pb-4">
                    {m.instructions ? (
                      <p className="whitespace-pre-line text-sm text-muted-foreground">{m.instructions}</p>
                    ) : null}
                    {finish}
                  </div>
                ) : null}
              </div>
            );
          })}
          </div>
          {fields.providerSlug.errors ? (
            <FieldErrors id={fields.providerSlug.errorId} errors={fields.providerSlug.errors} />
          ) : null}
        </fieldset>
      )}
    </form>
  );
}

// Saved cards belong to one provider, so the list follows the method picked
function SavedCards({ cards, field }: { cards: SavedPaymentMethod[]; field: { name: string } }) {
  const t = useTranslations("checkout.payment");
  if (cards.length === 0) return null;
  return (
    <fieldset className="grid gap-2">
      <legend className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("card")}</legend>
      {cards.map((c) => (
        <ChoiceCard key={c.id} name={field.name} value={c.id} defaultChecked={false}>
          {c.brand || t("cardFallback")} •••• {c.last4}
          {c.expMonth > 0 ? (
            <span className="text-muted-foreground">
              {" "}
              ({t("expires", { month: String(c.expMonth).padStart(2, "0"), year: c.expYear })})
            </span>
          ) : null}
        </ChoiceCard>
      ))}
      <ChoiceCard name={field.name} value="" defaultChecked>
        {t("newCard")}
      </ChoiceCard>
    </fieldset>
  );
}

// The store picks how the terms are accepted - a box to tick (many EU markets), a line saying placing the order accepts
// them, or neither
function Consent({
  consent,
  field,
}: {
  consent: CheckoutSettings["termsConsent"];
  field: { name: string; errors?: string[]; errorId?: string };
}) {
  const t = useTranslations("checkout.payment");
  if (consent === "off") return null;
  const links = {
    terms: (chunks: React.ReactNode) => (
      <Link href="/terms" target="_blank" rel="noreferrer" className="underline underline-offset-2">
        {chunks}
      </Link>
    ),
    privacy: (chunks: React.ReactNode) => (
      <Link href="/privacy" target="_blank" rel="noreferrer" className="underline underline-offset-2">
        {chunks}
      </Link>
    ),
  };
  if (consent === "notice") {
    return <p className="text-xs text-muted-foreground">{t.rich("tosNotice", links)}</p>;
  }
  return (
    <div className="grid gap-1">
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          name={field.name}
          aria-invalid={field.errors ? true : undefined}
          aria-describedby={field.errors ? field.errorId : undefined}
          className="mt-0.5 size-4 rounded accent-primary"
        />
        <span>{t.rich("tos", links)}</span>
      </label>
      {field.errors ? <FieldErrors id={field.errorId} errors={field.errors} /> : null}
    </div>
  );
}

function GiftCards({ giftCards, currency }: { giftCards: CheckoutState["giftCards"]; currency: Currency }) {
  const t = useTranslations("checkout.giftCards");
  const [open, setOpen] = useState(giftCards.length > 0);
  const { form, fields, formAction, onSubmit, pending } = useActionForm(applyGiftCardAction, giftCardSchema);
  const code = fields.code;
  return (
    <section className="grid gap-2">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("title")}</h3>
      {giftCards.length > 0 ? (
        <ul className="grid gap-2">
          {giftCards.map((g) => (
            <li
              key={g.code}
              className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2 text-sm"
            >
              <span className="min-w-0">
                <span className="font-mono text-xs uppercase tracking-wider">{g.code}</span>
                <span className="block text-xs text-muted-foreground">
                  {t("covers", {
                    applied: formatPrice(g.appliedCents, currency),
                    balance: formatPrice(g.balanceCents, currency),
                  })}
                </span>
              </span>
              <form action={removeGiftCardAction}>
        <CheckoutCartFields />
                <input type="hidden" name="code" value={g.code} />
                <button type="submit" className="text-xs text-muted-foreground hover:text-destructive">
                  {t("remove")}
                </button>
              </form>
            </li>
          ))}
        </ul>
      ) : null}
      {open ? (
        <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-1">
        <CheckoutCartFields />
          <FormErrors errors={form.errors} />
          <div className="flex gap-2">
            <input
              type="text"
              key={code.key}
              name={code.name}
              defaultValue={typeof code.initialValue === "string" ? code.initialValue : ""}
              placeholder="GC-XXXX-XXXX-XXXX"
              aria-label={t("title")}
              aria-invalid={code.errors ? true : undefined}
              aria-describedby={code.errors ? code.errorId : undefined}
              autoComplete="off"
              className={`${inputClass} flex-1 font-mono`}
            />
            <button
              type="submit"
              disabled={pending}
              className="rounded-md border border-border px-4 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
            >
              {pending ? t("applying") : t("apply")}
            </button>
          </div>
          {code.errors ? <FieldErrors id={code.errorId} errors={code.errors} /> : null}
        </form>
      ) : (
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="justify-self-start text-xs underline underline-offset-2"
        >
          {t("add")}
        </button>
      )}
    </section>
  );
}

function StoreCredit({ using, balance, currency }: { using: boolean; balance: number; currency: Currency }) {
  const t = useTranslations("checkout.storeCredit");
  return (
    <form action={setStoreCreditAction} className="flex items-center justify-between gap-3 rounded-lg border border-border p-3 text-sm">
        <CheckoutCartFields />
      <input type="hidden" name="use" value={using ? "0" : "1"} />
      <span>
        {t("available", { amount: formatPrice(balance, currency) })}
        <span className="mt-0.5 block text-xs text-muted-foreground">{t("hint")}</span>
      </span>
      <button
        type="submit"
        className="shrink-0 rounded-md border border-border px-3 py-1.5 text-xs font-semibold transition-colors hover:bg-muted"
      >
        {using ? t("stop") : t("use")}
      </button>
    </form>
  );
}
