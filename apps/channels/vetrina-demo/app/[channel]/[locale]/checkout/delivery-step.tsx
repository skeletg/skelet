"use client";

import { useTranslations } from "next-intl";
import { createContext, useCallback, useContext, useEffect, useId, useRef, useState } from "react";

import { Field, FieldErrors, FormErrors } from "@/app/components/account/AccountUI";
import { Link } from "@/app/components/LocaleLink";
import {
  saveDeliveryPointAction,
  saveDestinationStep,
  saveGroupDeliveryAction,
  saveSplitDeliveryAction,
} from "@/app/lib/checkout/actions";
import { groupRequirements, type Requirement } from "@/app/lib/checkout/asks";
import { choiceKey, choiceLabel } from "@/app/lib/checkout/choice";
import type {
  CheckoutDestination,
  CheckoutState,
  DeliveryChoice,
  DeliveryGroup,
  DeliveryPoint,
  DeliveryPointsResult,
} from "@/app/lib/checkout/read";
import {
  deliveryPointSchema,
  destinationSchema,
  groupDeliverySchema,
  splitDeliverySchema,
} from "@/app/lib/checkout/schemas";
import type { CountryOption } from "@/app/lib/countries/read";
import { formatPrice } from "@/app/lib/format";
import { useActionForm } from "@/app/lib/forms/use-action-form";

import { ChoiceCard, CountrySelect } from "./fields";

// All delivery forms share a pending guard so a group change cannot race a point or split change.
const DeliveryPending = createContext<(id: string, pending: boolean) => void>(() => {});
function useDeliveryPending(pending: boolean) {
  const report = useContext(DeliveryPending);
  const id = useId();
  useEffect(() => {
    report(id, pending);
    return () => report(id, false);
  }, [id, pending, report]);
}

type Currency = string | { code: string; symbol?: string; symbolPosition?: string };

// The one choice of a group that arrives soonest, when the carriers say - by arrival date, not days quoted, since a
// carrier that delivers on Saturday can beat a faster one that doesn't. A tie marks none, since "fastest" of two equals
// tells the shopper nothing
export function fastestChoice(choices: DeliveryChoice[]): string | null {
  const timed = choices.filter((c) => c.shipTo === "CUSTOMER" && c.estimatedArrival);
  if (timed.length < 2) return null;
  // ISO dates order as strings
  const soonestDate = timed.map((c) => c.estimatedArrival!).sort()[0];
  const soonest = timed.filter((c) => c.estimatedArrival === soonestDate);
  return soonest.length === 1 ? choiceKey(soonest[0]) : null;
}

export type Thresholds = {
  show: boolean;
  subtotalCents: number;
  minOrderCents: number;
  freeShippingCents: number;
};

// A cart line as the delivery step names it
export type LineInfo = { id: string; name: string; quantity: number; imageUrl?: string | null };

// How the order arrives: a postcode first, then each group of lines with what it can choose - one group for most carts,
// several when the lines allow different deliveries (a fridge to the door, the rest to a PO box) - and the store or PO box
// the chosen delivery goes to. Every save re-renders the step from the cart's requirements, so it always asks exactly what
// placement will check
export function DeliveryStep({
  checkout,
  suggested,
  countries,
  lines,
  arrivals,
  points,
  pointSearch,
  thresholds,
  notice,
  currency,
}: {
  checkout: CheckoutState;
  // Prefill when nothing is saved yet - a signed-in customer's default address, else the store's country
  suggested: { countryCode: string; postalCode: string };
  countries: CountryOption[];
  lines: Record<string, LineInfo>;
  // choiceKey -> "Thu 3 Oct", formatted on the server so the date can't differ between render and hydration
  arrivals: Record<string, string>;
  // Per group key - fetched for groups whose chosen delivery asks for a point
  points: Record<string, DeliveryPointsResult>;
  pointSearch: { group: string; query: string };
  thresholds: Thresholds;
  notice: boolean;
  currency: Currency;
}) {
  const t = useTranslations("checkout.delivery");
  const [pendingForms, setPendingForms] = useState<Set<string>>(() => new Set());
  const reportPending = useCallback((id: string, pending: boolean) => {
    setPendingForms((prev) => {
      if (prev.has(id) === pending) return prev;
      const next = new Set(prev);
      if (pending) next.add(id); else next.delete(id);
      return next;
    });
  }, []);
  const busy = pendingForms.size > 0;
  const destination = checkout.destination;
  const belowMinimum = thresholds.minOrderCents > 0 && thresholds.subtotalCents < thresholds.minOrderCents;
  const groups = checkout.deliveryGroups;
  const undeliverable = checkout.requirements.filter((r) => r.scope === "order" && r.type === "delivery_option");
  const ready = !!destination && !belowMinimum && undeliverable.length === 0 && groups.every((g) => !groupRequirements(checkout.requirements, g.key).filter((r) => r.type === "delivery_option" || r.type === "point").some(isOpen));
  return (
    <DeliveryPending.Provider value={reportPending}>
      <fieldset disabled={busy} aria-busy={busy} className="grid min-w-0 gap-5">
        {notice ? (
          <p role="status" className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
            {t("changedNotice")}
          </p>
        ) : null}
        <DestinationForm current={destination} suggested={suggested} countries={countries} />
        {thresholds.show && belowMinimum ? <MinimumOrder thresholds={thresholds} currency={currency} /> : null}

        {destination && !belowMinimum ? (
          <>
            {undeliverable.map((r) => (
              <p key={r.label} role="alert" className="rounded-lg border border-destructive/40 p-3 text-sm text-destructive">
                {r.message}
              </p>
            ))}
            {checkout.digitalLineIds.length > 0 ? (
              <DigitalNote names={checkout.digitalLineIds.map((id) => lines[id]?.name ?? "")} />
            ) : null}
            {groups.length > 1 && !checkout.canSplit ? <p className="text-sm text-muted-foreground">{t("automaticGroups")}</p> : null}
            {checkout.canSplit ? <SplitForm split={checkout.splitDelivery} /> : null}
            {groups.map((g, i) => (
              <GroupCard
                key={g.key}
                group={g}
                index={i}
                total={groups.length}
                lines={lines}
                requirements={groupRequirements(checkout.requirements, g.key)}
                arrivals={arrivals}
                points={points[g.key] ?? { points: [], unavailable: false }}
                search={pointSearch.group === g.key ? pointSearch.query : ""}
                currency={currency}
              />
            ))}
            {groups.some((g) => g.selection) ? (
              <p className="flex justify-between gap-3 text-sm font-medium">
                <span>{t("deliveryTotal")}</span>
                <span>{formatPrice(groups.reduce((sum, g) => sum + (g.selection?.priceCents ?? 0), 0), currency)}</span>
              </p>
            ) : null}
            {thresholds.show && thresholds.freeShippingCents > 0 ? (
              <p className="text-xs text-muted-foreground">{t("thresholdHint", { amount: formatPrice(thresholds.freeShippingCents, currency) })}</p>
            ) : null}
          </>
        ) : null}
        <p role="status" className="text-sm text-muted-foreground">
          {busy ? t("saving") : ready ? t("reviewDelivery") : t("completeDelivery")}
        </p>
        {ready && !busy ? (
          <Link href="/checkout?step=details" className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-8 text-sm font-semibold text-primary-foreground">
            {t("continueToDetails")}
          </Link>
        ) : (
          <button type="button" disabled className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-8 text-sm font-semibold text-primary-foreground opacity-50">
            {t("continueToDetails")}
          </button>
        )}
      </fieldset>
    </DeliveryPending.Provider>
  );
}

const isOpen = (r: Requirement) => r.status === "MISSING" || r.status === "INVALID";

function DestinationForm({
  current,
  suggested,
  countries,
}: {
  current: CheckoutDestination | null;
  suggested: { countryCode: string; postalCode: string };
  countries: CountryOption[];
}) {
  const t = useTranslations("checkout.delivery");
  const { form, fields, formAction, onSubmit, pending } = useActionForm(saveDestinationStep, destinationSchema, {
    defaultValue: {
      countryCode: current?.countryCode ?? suggested.countryCode,
      postalCode: current?.postalCode ?? suggested.postalCode,
    },
  });
  useDeliveryPending(pending);
  // One country needs no picker - it posts as a hidden field
  const single = countries.length === 1 ? countries[0].code : null;
  return (
    <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-3">
      <p className="text-sm text-muted-foreground">{t("postcodeIntro")}</p>
      <FormErrors errors={form.errors} />
      <div className={`grid items-start gap-3 ${single ? "sm:grid-cols-[1fr_auto]" : "sm:grid-cols-[1fr_1fr_auto]"}`}>
        {single ? (
          <input type="hidden" name={fields.countryCode.name} value={single} />
        ) : (
          <CountrySelect field={fields.countryCode} countries={countries} autoComplete="shipping country" />
        )}
        <Field label={t("postcode")} field={fields.postalCode} autoComplete="shipping postal-code" required />
        {/* An empty label row above the button lines it up with the inputs, and keeps it there when an error opens
            below the postcode. Same border, padding and text size as the inputs, so the same height */}
        <div className="flex flex-col gap-1.5 text-sm">
          <span aria-hidden className="invisible hidden font-medium sm:block">
            {t("postcode")}
          </span>
          <button
            type="submit"
            disabled={pending}
            className="inline-flex items-center justify-center rounded-lg border border-border px-5 py-2 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
          >
            {pending ? t("finding") : current ? t("update") : t("showOptions")}
          </button>
        </div>
      </div>
    </form>
  );
}

function DigitalNote({ names }: { names: string[] }) {
  const t = useTranslations("checkout.delivery");
  return (
    <div className="rounded-lg border border-border p-3 text-sm">
      <p className="font-medium">{t("digitalTitle")}</p>
      <p className="mt-0.5 text-muted-foreground">{names.filter(Boolean).join(", ")}</p>
    </div>
  );
}

// Offered when one delivery could cover everything but the lines also allow others - "together" is one choice,
// "split" asks per group so the small items can go to a PO box while the fridge comes to the door
function SplitForm({ split }: { split: boolean }) {
  const t = useTranslations("checkout.delivery");
  const ref = useRef<HTMLFormElement>(null);
  const { form, fields, formAction, onSubmit, pending } = useActionForm(saveSplitDeliveryAction, splitDeliverySchema, {
    defaultValue: { split: split ? "split" : "together" },
  });
  useDeliveryPending(pending);
  return (
    <form ref={ref} id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-2">
      <FormErrors errors={form.errors} />
      <fieldset className="grid gap-2 sm:grid-cols-2" onChange={() => ref.current?.requestSubmit()}>
        <legend className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("splitLegend")}
        </legend>
        <ChoiceCard name={fields.split.name} value="together" defaultChecked={!split}>
          <span className="font-medium">{t("together")}</span>
          <span className="mt-0.5 block text-xs text-muted-foreground">{t("togetherHint")}</span>
        </ChoiceCard>
        <ChoiceCard name={fields.split.name} value="split" defaultChecked={split}>
          <span className="font-medium">{t("split")}</span>
          <span className="mt-0.5 block text-xs text-muted-foreground">{t("splitHint")}</span>
        </ChoiceCard>
      </fieldset>
      <noscript><button type="submit" className="text-sm underline">{t("choose")}</button></noscript>
    </form>
  );
}

function GroupCard({
  group,
  index,
  total,
  lines,
  requirements,
  arrivals,
  points,
  search,
  currency,
}: {
  group: DeliveryGroup;
  index: number;
  total: number;
  lines: Record<string, LineInfo>;
  requirements: Requirement[];
  arrivals: Record<string, string>;
  points: DeliveryPointsResult;
  search: string;
  currency: Currency;
}) {
  const t = useTranslations("checkout.delivery");
  const sel = group.selection;
  const asked = requirements.filter((r) => r.type !== "delivery_option");
  return (
    <section className="grid gap-4 rounded-lg border border-border p-4" aria-label={t("groupTitle", { n: index + 1, total })}>
      {total > 1 ? (
        <header className="grid gap-1">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("groupTitle", { n: index + 1, total })}
          </h3>
          <p className="text-sm">
            {group.lineIds
              .map((id) => lines[id])
              .filter(Boolean)
              .map((l) => (l.quantity > 1 ? `${l.quantity} × ${l.name}` : l.name))
              .join(", ")}
          </p>
        </header>
      ) : null}

      <ChoicesForm group={group} arrivals={arrivals} currency={currency} />

      {/* The collector is asked with the contact details, in the next step */}
      {sel
        ? asked
            .filter((r) => r.type === "point")
            .map((r) => (
              <PointForm key={sel.optionId + sel.methodCode} groupKey={group.key} requirement={r} result={points} search={search} selected={sel.point ?? null} />
            ))
        : null}

      {sel && group.shipments.length > 1 ? <Shipments group={group} currency={currency} /> : null}
    </section>
  );
}

// Every option and carrier method the group can take, cheapest first - picking one saves it at once, so what it asks
// next opens straight below
function ChoicesForm({ group, arrivals, currency }: { group: DeliveryGroup; arrivals: Record<string, string>; currency: Currency }) {
  const t = useTranslations("checkout.delivery");
  const ref = useRef<HTMLFormElement>(null);
  const sel = group.selection;
  const initial = sel ? choiceKey(sel) : "";
  const { form, fields, formAction, onSubmit, pending } = useActionForm(saveGroupDeliveryAction, groupDeliverySchema, {
    defaultValue: { groupKey: group.key, selection: initial },
  });
  useDeliveryPending(pending);
  if (group.choices.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  }
  const ordered = group.choices.slice().sort((a, b) => a.priceCents - b.priceCents);
  const fastest = fastestChoice(group.choices);
  return (
    <form ref={ref} id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-2">
      <input type="hidden" name={fields.groupKey.name} value={group.key} />
      <FormErrors errors={form.errors} />
      <fieldset onChange={() => ref.current?.requestSubmit()} disabled={pending}>
        <legend className="sr-only">{t("legend")}</legend>
        <ul className="space-y-2" aria-live="polite">
          {ordered.map((c) => {
            const key = choiceKey(c);
            return (
              <li key={key}>
                <ChoiceCard
                  name={fields.selection.name}
                  value={key}
                  defaultChecked={initial === key}
                  aside={c.priceCents === 0 ? t("free") : formatPrice(c.priceCents, currency)}
                >
                  {choiceLabel(c, group.choices).map((part, i) =>
                    i === 0 ? (
                      <span key={i} className="font-medium">
                        {part}
                      </span>
                    ) : (
                      <span key={i} className="text-muted-foreground">
                        {" "}
                        · {part}
                      </span>
                    ),
                  )}
                  {fastest === key ? (
                    <span className="ml-2 rounded-full bg-foreground px-2 py-0.5 text-[11px] font-semibold text-background">
                      {t("fastest")}
                    </span>
                  ) : null}
                  {c.description ? <span className="mt-0.5 block text-xs text-muted-foreground">{c.description}</span> : null}
                  {arrivals[key] ? (
                    <span className="mt-0.5 block text-xs text-muted-foreground">{t("arrives", { date: arrivals[key] })}</span>
                  ) : null}
                </ChoiceCard>
              </li>
            );
          })}
        </ul>
      </fieldset>
      {fields.selection.errors ? <FieldErrors id={fields.selection.errorId} errors={fields.selection.errors} /> : null}
      <noscript>
        <button type="submit" className="text-sm underline">
          {t("choose")}
        </button>
      </noscript>
    </form>
  );
}

function BlockHeading({ requirement }: { requirement: Requirement }) {
  return (
    <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
      {requirement.label}
      {requirement.status === "SATISFIED" ? <span className="ml-2 normal-case tracking-normal text-emerald-700">✓</span> : null}
    </h4>
  );
}

// The stores or PO boxes the chosen delivery can go to - saved on pick, re-read from its source by the backend
function PointForm({
  groupKey,
  requirement,
  result,
  search,
  selected,
}: {
  groupKey: string;
  requirement: Requirement;
  result: DeliveryPointsResult;
  search: string;
  selected: DeliveryPoint | null;
}) {
  const t = useTranslations("checkout.delivery");
  const ref = useRef<HTMLFormElement>(null);
  const { form, fields, formAction, onSubmit, pending } = useActionForm(saveDeliveryPointAction, deliveryPointSchema, {
    defaultValue: { groupKey, pointId: selected?.id ?? "" },
  });
  useDeliveryPending(pending);
  const points = selected && !result.points.some((p) => p.id === selected.id) ? [selected, ...result.points] : result.points;
  return (
    <div className="grid gap-3">
      <form method="get" className="flex flex-wrap items-end gap-2">
        <input type="hidden" name="step" value="delivery" />
        <input type="hidden" name="pointGroup" value={groupKey} />
        <label className="grid flex-1 gap-1 text-sm">
          {t("pointSearch")}
          <input key={search} name="pointSearch" defaultValue={search} className="rounded-md border border-border px-3 py-2" />
        </label>
        <button type="submit" className="rounded-md border border-border px-3 py-2 text-sm">{t("search")}</button>
        {result.unavailable ? <button type="submit" className="text-sm underline">{t("retry")}</button> : null}
      </form>
      {result.unavailable ? <p role="alert" className="text-sm text-destructive">{t("pointsUnavailable")}</p> : result.points.length === 0 ? <p role="status" className="text-sm text-muted-foreground">{t("noPoints")}</p> : null}
      <form ref={ref} id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-2">
        <input type="hidden" name={fields.groupKey.name} value={groupKey} />
        <BlockHeading requirement={requirement} />
        <FormErrors errors={form.errors} />
        {points.length > 0 ? (
          <fieldset className="grid gap-2 sm:grid-cols-2" onChange={() => ref.current?.requestSubmit()} disabled={pending}>
            <legend className="sr-only">{requirement.label}</legend>
            {points.map((p) => (
              <ChoiceCard key={p.id} name={fields.pointId.name} value={p.id} defaultChecked={selected?.id === p.id}>
                <span className="font-medium">{p.name}</span>
                <span className="mt-0.5 block text-xs text-muted-foreground">
                  {[p.street1, p.city, p.postalCode].filter(Boolean).join(", ")}
                </span>
                {p.hours ? <span className="mt-0.5 block text-xs text-muted-foreground">{p.hours}</span> : null}
                {p.instructions ? <span className="mt-1 block text-xs text-muted-foreground">{p.instructions}</span> : null}
              </ChoiceCard>
            ))}
          </fieldset>
        ) : null}
        <noscript><button type="submit" className="text-sm underline">{t("choose")}</button></noscript>
      </form>
    </div>
  );
}

// More than one package - each leaves from its own warehouse or store and is priced on its own, the total is their sum
function Shipments({ group, currency }: { group: DeliveryGroup; currency: Currency }) {
  const t = useTranslations("checkout.delivery");
  return (
    <div className="grid gap-1 border-t border-border pt-3 text-xs text-muted-foreground">
      <p className="font-medium text-foreground">{t("packages", { count: group.shipments.length })}</p>
      <ul className="divide-y divide-border">
        {group.shipments.map((s, i) => (
          <li key={i} className="flex justify-between gap-3 py-1">
            <span>
              {t("package", { n: i + 1 })}
              {s.originName ? ` · ${t("from", { origin: s.originName })}` : ""} · {t("items", { count: s.lineIds.length })}
            </span>
            {s.priceCents != null ? (
              <span className="tabular-nums">{s.priceCents === 0 ? t("free") : formatPrice(s.priceCents, currency)}</span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

// Below the store's minimum, a goal rather than a refusal at the last step - how much more, and the way back to the cart
function MinimumOrder({ thresholds, currency }: { thresholds: Thresholds; currency: Currency }) {
  const t = useTranslations("checkout.delivery");
  const remaining = thresholds.minOrderCents - thresholds.subtotalCents;
  const pct = Math.max(0, Math.min(100, Math.round((thresholds.subtotalCents / thresholds.minOrderCents) * 100)));
  return (
    <div className="rounded-lg border border-border p-4 text-sm">
      <p className="font-semibold">{t("minimumTitle", { amount: formatPrice(thresholds.minOrderCents, currency) })}</p>
      <div
        className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-border"
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={t("minimumProgress")}
      >
        <div className="h-full rounded-full bg-amber-500" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-2 text-muted-foreground">
        {t.rich("minimumRemaining", {
          amount: () => <span className="font-medium text-foreground">{formatPrice(remaining, currency)}</span>,
        })}
      </p>
      <Link href="/cart" className="mt-2 inline-block text-xs underline underline-offset-2">
        {t("backToCart")}
      </Link>
    </div>
  );
}
