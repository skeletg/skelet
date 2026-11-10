"use client";

import { CheckoutCartFields } from "@/app/components/offer/CheckoutCartContext";

import { useTranslations } from "next-intl";
import { startTransition, useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";

import { FieldErrors, FormErrors, inputClass } from "@/app/components/account/AccountUI";
import { Link } from "@/app/components/LocaleLink";
import {
  applyGiftCardAction,
  finalizePlacedPaymentAction,
  placeOrderAction,
  removeGiftCardAction,
  setStoreCreditAction,
} from "@skelet/storefront-next/checkout/actions";
import { getPaymentUI, type PaymentHandle } from "@/app/plugins/registry";
import type { PendingPayment } from "@skelet/storefront-core/checkout/actions";
import type { CheckoutState, PaymentMethodOption } from "@skelet/storefront-next/checkout/read";
import { giftCardSchema, paymentStepSchema } from "@skelet/storefront-core/checkout/schemas";
import type { CheckoutSettings } from "@skelet/storefront-core/checkout/settings";
import { formatPrice } from "@skelet/storefront-core/format";
import { useActionForm } from "@skelet/storefront-react/forms/use-action-form";
import type { SavedPaymentMethod } from "@skelet/storefront-next/payment-methods/actions";

import { ChoiceCard } from "./fields";

type Currency = string | { code: string; symbol?: string; symbolPosition?: string };

// How long a placed order's payment step may run before the shopper is offered their order page anyway
const STALL_MS = 60_000;

export function PaymentStep({
  methods,
  checkout,
  note,
  savedCards,
  storeCreditBalance,
  signedIn,
  emailOptIn,
  consent,
  currency,
}: {
  methods: PaymentMethodOption[];
  checkout: CheckoutState;
  note: string;
  savedCards: SavedPaymentMethod[];
  storeCreditBalance: number;
  signedIn: boolean;
  emailOptIn: boolean; // false on a guest offer opened while signed in - the tick would belong to the recipient
  consent: CheckoutSettings["termsConsent"];
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
        emailOptIn={emailOptIn}
        consent={consent}
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
  emailOptIn,
  consent,
  currency,
}: {
  methods: PaymentMethodOption[];
  checkout: CheckoutState;
  note: string;
  savedCards: SavedPaymentMethod[];
  emailOptIn: boolean;
  consent: CheckoutSettings["termsConsent"];
  currency: Currency;
}) {
  const t = useTranslations("checkout.payment");
  // Gift cards and store credit covering the total leave nothing for a method to charge
  const covered = checkout.totals.amountDueCents <= 0;
  const provider = covered ? "" : (checkout.paymentProviderSlug ?? methods[0]?.providerSlug ?? "");
  const schema = useMemo(() => paymentStepSchema({ requireMethod: !covered, consent }), [covered, consent]);
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(placeOrderAction, schema, {
    defaultValue: { providerSlug: provider, savedPaymentMethodId: "", note },
  });
  const picked = typeof fields.providerSlug.value === "string" ? fields.providerSlug.value : provider;
  const pickedUI = covered ? undefined : getPaymentUI(picked);
  // The gateway's card fields show while its method is picked and no saved card stands in for them
  const savedCard = typeof fields.savedPaymentMethodId.value === "string" && fields.savedPaymentMethodId.value !== "";
  const ui = savedCard ? undefined : pickedUI;
  const [registered, setRegistered] = useState<{ provider: string; handle: PaymentHandle } | null>(null);
  const uiProvider = ui?.provider;
  // Stable per provider, so fields that register from an effect don't loop - a late null from the last provider's
  // fields never unregisters the next one's
  const register = useCallback(
    (h: PaymentHandle | null) => {
      if (!uiProvider) return;
      setRegistered((cur) => (h ? { provider: uiProvider, handle: h } : cur?.provider === uiProvider ? null : cur));
    },
    [uiProvider],
  );
  const handle = ui && registered?.provider === ui.provider ? registered.handle : null;
  // A saved card mounts no fields, so only the gateway's own registration can run its step
  const challenge = handle?.challenge ?? pickedUI?.challenge;
  const canRunStep = ui !== undefined || pickedUI?.challenge !== undefined;
  const [paying, setPaying] = useState(false);
  const [payError, setPayError] = useState<string | null>(null);

  // The card fields make their token right before placing, so it is fresh and never sits in the form
  const pay = (event: FormEvent<HTMLFormElement>) => {
    if (!ui) return onSubmit(event);
    form.onSubmit(event);
    if (event.defaultPrevented) return;
    event.preventDefault();
    if (!handle) return; // the fields are still loading - Pay stays down until they register
    const data = new FormData(event.currentTarget, (event.nativeEvent as SubmitEvent).submitter);
    setPayError(null);
    setPaying(true);
    handle
      .tokenize()
      .then((token) => {
        if (token) data.set("paymentToken", token);
        startTransition(() => formAction(data));
      })
      .catch((err: unknown) => setPayError(err instanceof Error && err.message ? err.message : t("cardFailed")))
      .finally(() => setPaying(false));
  };

  // The order is placed and the gateway wants its in-page step - run it, then let the gateway say how it went
  const pendingPayment = lastResult?.payment;
  const [stalled, setStalled] = useState(false);
  const started = useRef<PendingPayment | null>(null);
  useEffect(() => {
    if (!pendingPayment || started.current === pendingPayment) return;
    started.current = pendingPayment;
    const { clientSecret, ...placed } = pendingPayment;
    // The order exists either way, so a step or finalize that hangs or fails still leaves a way to it
    setTimeout(() => setStalled(true), STALL_MS);
    (challenge ? challenge(clientSecret) : Promise.resolve())
      .catch(() => undefined) // a cancelled or failed step is the gateway's to report, not the page's
      .then(() =>
        startTransition(async () => {
          try {
            await finalizePlacedPaymentAction(placed);
          } catch {
            setStalled(true);
          }
        }),
      );
  }, [pendingPayment, challenge]);
  // Placing, making the token, or finishing the gateway's step - the button stays down until the order page loads
  const busy = pending || paying || pendingPayment !== undefined || (ui !== undefined && !handle);

  if (!covered && methods.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  }

  // What follows the chosen method, rendered once inside it: saved cards, the gateway's fields, the note, consent and the
  // Pay button
  const finish = (
    <div className="grid gap-4">
      <SavedCards cards={savedCards.filter((c) => c.providerSlug === picked)} field={fields.savedPaymentMethodId} />
      {ui ? (
        <>
          <ui.Fields
            key={ui.provider}
            amountCents={checkout.totals.amountDueCents}
            currency={typeof currency === "string" ? currency : currency.code}
            register={register}
          />
          {payError ? <p role="alert" className="text-sm text-destructive">{payError}</p> : null}
        </>
      ) : null}
      {canRunStep ? <input type="hidden" name={fields.paymentUi.name} value="on" /> : null}
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
      {emailOptIn ? (
        <label className="flex items-start gap-2 text-sm">
          <input type="checkbox" name={fields.marketingOptIn.name} className="mt-0.5 size-4 rounded accent-primary" />
          <span>{t("marketingOptIn")}</span>
        </label>
      ) : null}
      <Consent consent={consent} field={fields.tos} />
      <button
        type="submit"
        disabled={busy}
        className="inline-flex h-12 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
      >
        {busy
          ? t("placing")
          : covered
            ? t("placeCovered")
            : t("pay", { amount: formatPrice(checkout.totals.amountDueCents, currency) })}
      </button>
      {stalled && pendingPayment ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t("paymentStalled")}{" "}
          <Link href={placedOrderHref(pendingPayment)} className="underline underline-offset-2">
            {t("openOrder")}
          </Link>
        </p>
      ) : null}
    </div>
  );

  return (
    <form id={form.id} onSubmit={pay} action={formAction} noValidate className="grid gap-4">
        <CheckoutCartFields />
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

// Where the order page is for a payment the gateway hasn't settled yet - a guest's opens with its email
function placedOrderHref({ orderId, email }: PendingPayment): string {
  const id = encodeURIComponent(orderId);
  return email ? `/orders/${id}?email=${encodeURIComponent(email)}&payment=incomplete` : `/account/orders/${id}?payment=incomplete`;
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
