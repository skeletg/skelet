"use client";

import {
  Fragment,
  useActionState,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Link } from "@/app/components/LocaleLink";

import {
  addAddressAction,
  finalizeOrderPaymentAction,
  placeOrderAction,
  type CheckoutResult,
} from "@/app/lib/checkout/actions";
import { setCartEmailAction } from "@/app/lib/cart/actions";
import type { CountryOption, CountrySettings, RegionOption } from "@/app/lib/countries/read";
import type { Address } from "@/app/lib/customer/read";
import { formatPrice } from "@/app/lib/format";
import { subdivisionsFor } from "@/app/lib/states";

type ShippingQuote = {
  carrierSlug: string;
  carrierName: string;
  code: string;
  name: string;
  priceCents: number;
};

const quoteKey = (q: { carrierSlug: string; code: string }) =>
  `${q.carrierSlug}::${q.code}`;

const PICKUP_CARRIER = "pickup";
const isPickupQuote = (q: { carrierSlug: string }) =>
  q.carrierSlug === PICKUP_CARRIER;
// Pickup codes are "loc-<slug>" - see the backend PickupCarrier
const pickupSlug = (q: { code: string }) => q.code.replace(/^loc-/, "");

type PaymentMethodOption = {
  providerSlug: string;
  displayName: string;
  sortOrder: number;
  instructions: string;
};

type StepId = "customer" | "shipping" | "billing" | "review";

export function CheckoutForm({
  customerEmail,
  storeCreditBalance,
  savedPaymentMethods,
  addresses,
  defaultBillingId,
  defaultShippingId,
  initialShippingQuotes,
  pickupLocations,
  countries,
  regionsByCountry,
  countrySettings,
  currency,
  initialNote,
}: {
  customerEmail: string;
  storeCreditBalance: number;
  savedPaymentMethods: {
    id: string;
    providerSlug: string;
    brand: string;
    last4: string;
    expMonth: number;
    expYear: number;
  }[];
  addresses: Address[];
  defaultBillingId: string | null;
  defaultShippingId: string | null;
  initialShippingQuotes: ShippingQuote[];
  pickupLocations: Array<{
    slug: string;
    name: string;
    line1: string;
    city: string;
    postalCode: string;
    countryCode: string;
    instructions: string;
  }>;
  countries: CountryOption[];
  regionsByCountry: Record<string, RegionOption[]>;
  countrySettings: CountrySettings;
  currency: string | { code: string; symbol?: string; symbolPosition?: string };
  initialNote: string;
}) {
  const isGuest = customerEmail === "";

  const [email, setEmail] = useState(customerEmail);

  const firstId = addresses[0]?.id ?? "";
  const [shippingId, setShippingId] = useState(defaultShippingId ?? firstId);

  const [billingId, setBillingId] = useState(defaultBillingId ?? firstId);
  const [sameAsBilling, setSameAsBilling] = useState(
    billingId === shippingId && billingId !== "",
  );

  const [showNewAddress, setShowNewAddress] = useState(false);

  const [shippingQuotes, setShippingQuotes] = useState<ShippingQuote[]>(
    initialShippingQuotes,
  );
  const [shippingLoading, setShippingLoading] = useState(false);
  // The customer's explicit pick - it only counts while that quote is still on offer, so
  // the effective selection is resolved during render just below
  const [pickedShippingKey, setPickedShippingKey] = useState<string>("");

  const pickupBySlug = new Map(pickupLocations.map((l) => [l.slug, l]));
  const orderedQuotes = shippingQuotes
    .slice()
    .sort(
      (a, b) =>
        Number(isPickupQuote(a)) - Number(isPickupQuote(b)) ||
        a.priceCents - b.priceCents,
    );
  const bothKindsOffered =
    orderedQuotes.some(isPickupQuote) && orderedQuotes.some((q) => !isPickupQuote(q));
  // Falls back to the cheapest quote (pickup included, unlike orderedQuotes' display sort) whenever the pick isn't on offer - quotes reload
  // per country, and resolving here rather than in an effect avoids a render showing a stale or blank method
  const cheapestQuote = shippingQuotes
    .slice()
    .sort((a, b) => a.priceCents - b.priceCents)[0];
  const shippingSel = shippingQuotes.some((q) => quoteKey(q) === pickedShippingKey)
    ? pickedShippingKey
    : cheapestQuote
      ? quoteKey(cheapestQuote)
      : "";

  const renderQuote = (
    q: ShippingQuote,
    loc?: { name: string; line1: string; city: string; postalCode: string; instructions: string },
  ) => {
    const key = quoteKey(q);
    const checked = key === shippingSel;
    return (
      <li>
        <label
          className={`flex cursor-pointer items-start justify-between gap-3 rounded-lg border p-3 transition-colors ${
            checked ? "border-primary ring-1 ring-primary" : "border-border hover:bg-muted"
          }`}
        >
          <span className="flex items-start gap-2">
            <input
              type="radio"
              name="shippingMethodRadio"
              value={key}
              checked={checked}
              onChange={() => setPickedShippingKey(key)}
              className="mt-0.5 size-4 accent-primary"
            />
            <span>
              {loc ? (
                <>
                  <span className="font-medium">{loc.name}</span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">
                    {[loc.line1, loc.city, loc.postalCode].filter(Boolean).join(", ")}
                  </span>
                  {loc.instructions ? (
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {loc.instructions}
                    </span>
                  ) : null}
                </>
              ) : (
                <>
                  {q.name}
                  <span className="text-muted-foreground"> · {q.carrierName}</span>
                </>
              )}
            </span>
          </span>
          <span className="shrink-0 font-medium tabular-nums">
            {q.priceCents === 0 ? "Free" : formatPrice(q.priceCents, currency)}
          </span>
        </label>
      </li>
    );
  };

  const [paymentMethods, setPaymentMethods] = useState<PaymentMethodOption[]>(
    [],
  );
  const [paymentLoading, setPaymentLoading] = useState(false);
  // Same shape as the shipping pick above - explicit choice in state, effective one resolved here
  const [pickedProviderSlug, setPickedProviderSlug] = useState<string>("");
  const firstPaymentMethod = paymentMethods
    .slice()
    .sort((a, b) => a.sortOrder - b.sortOrder)[0];
  const paymentProviderSlug = paymentMethods.some(
    (m) => m.providerSlug === pickedProviderSlug,
  )
    ? pickedProviderSlug
    : firstPaymentMethod
      ? firstPaymentMethod.providerSlug
      : "";

  // ---- Guest path: ship-to country is what the customer is typing into the inline
  // shipping fieldset rather than a saved-address pick
  const [guestShippingCountry, setGuestShippingCountry] = useState("");
  const guestShippingCountryUpper = guestShippingCountry.trim().toUpperCase();
  const [guestShippingRegion, setGuestShippingRegion] = useState("");
  const [guestShippingPostal, setGuestShippingPostal] = useState("");

  const [customerNote, setCustomerNote] = useState(initialNote);
  const [giftCardCodes, setGiftCardCodes] = useState("");
  const [useStoreCredit, setUseStoreCredit] = useState(false);
  // "" = type a new card; otherwise the saved method id to charge
  const [savedMethodId, setSavedMethodId] = useState("");
  const [tosAccepted, setTosAccepted] = useState(false);

  const [currentStep, setCurrentStep] = useState<StepId>(
    isGuest ? "customer" : "shipping",
  );
  const [completed, setCompleted] = useState<Set<StepId>>(
    () => (isGuest ? new Set() : new Set<StepId>(["customer"])),
  );

  function stepState(id: StepId): "active" | "completed" | "pending" {
    if (currentStep === id) return "active";
    if (completed.has(id)) return "completed";
    return "pending";
  }

  function goTo(id: StepId) {
    setCurrentStep(id);
  }

  function completeAndAdvance(from: StepId, to: StepId) {
    setCompleted((prev) => {
      const next = new Set(prev);
      next.add(from);
      const order: StepId[] = ["customer", "shipping", "billing", "review"];
      const fromIdx = order.indexOf(from);
      const toIdx = order.indexOf(to);
      for (let i = fromIdx + 1; i < toIdx; i++) next.add(order[i]);
      return next;
    });
    setCurrentStep(to);
  }

  const shippingAddress = useMemo(
    () => addresses.find((a) => a.id === shippingId) ?? null,
    [addresses, shippingId],
  );
  const billingAddress = useMemo(
    () =>
      addresses.find(
        (a) => a.id === (sameAsBilling ? shippingId : billingId),
      ) ?? null,
    [addresses, shippingId, billingId, sameAsBilling],
  );
  const shippingCountry = isGuest
    ? guestShippingCountryUpper
    : shippingAddress?.countryCode ?? "";
  const shippingRegion = isGuest ? guestShippingRegion : shippingAddress?.state ?? "";
  const shippingPostal = (isGuest ? guestShippingPostal : shippingAddress?.postalCode ?? "").trim();

  useEffect(() => {
    if (!shippingCountry) {
      // Dropping quotes the moment the destination goes away, so a stale price can't be
      // shown (or submitted) for a country the customer already cleared
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setShippingQuotes([]);
      return;
    }
    let cancelled = false;
    setShippingLoading(true);
    const params = new URLSearchParams({ country: shippingCountry, region: shippingRegion, postal: shippingPostal });
    // Postal codes arrive a keystroke at a time, so wait for a pause before re-quoting
    const timer = setTimeout(() => {
      fetch(`/api/shipping-quotes?${params}`)
        .then((r) => r.json())
        .then((data: { quotes?: ShippingQuote[] }) => {
          if (cancelled) return;
          setShippingQuotes(data.quotes ?? []);
        })
        .catch(() => {
          if (!cancelled) setShippingQuotes([]);
        })
        .finally(() => {
          if (!cancelled) setShippingLoading(false);
        });
    }, 350);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [shippingCountry, shippingRegion, shippingPostal]);

  // Re-fetch eligible payment methods when the destination country changes (a method
  // may be restricted to specific countries)
  useEffect(() => {
    if (!shippingCountry) {
      // Same reason as the quotes above - a method restricted to the old country must not
      // stay selectable once the destination is cleared
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setPaymentMethods([]);
      return;
    }
    let cancelled = false;
    setPaymentLoading(true);
    fetch(`/api/payment-methods?country=${encodeURIComponent(shippingCountry)}`)
      .then((r) => r.json())
      .then((data: { methods?: PaymentMethodOption[] }) => {
        if (cancelled) return;
        setPaymentMethods(data.methods ?? []);
      })
      .catch(() => {
        if (!cancelled) setPaymentMethods([]);
      })
      .finally(() => {
        if (!cancelled) setPaymentLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [shippingCountry]);

  const [addAddrState, addAddrAction, addAddrPending] = useActionState<
    CheckoutResult | null,
    FormData
  >(addAddressAction, null);
  const [placeState, placeAction, placePending] = useActionState<
    CheckoutResult | null,
    FormData
  >(placeOrderAction, null);

  // 3DS/SCA follow-through: when placement returns requiresAction the order exists but
  // the gateway wants a client-side challenge
  const [scaMessage, setScaMessage] = useState("");
  const scaHandled = useRef(false);
  useEffect(() => {
    if (!placeState || !placeState.ok || !("requiresAction" in placeState)) return;
    if (scaHandled.current) return;
    scaHandled.current = true;
    const { orderId, redirectTo } = placeState.requiresAction;
    (async () => {
      // Running the challenge needs the gateway's own browser SDK, which lived in the
      // payment plugins - with none installed we can only ask the gateway if it settled
      setScaMessage("Confirming your payment…");
      await finalizeOrderPaymentAction(orderId);
      window.location.assign(redirectTo);
    })();
  }, [placeState]);

  const formRef = useRef<HTMLFormElement>(null);
  const paymentTokenRef = useRef<HTMLInputElement>(null);

  const providerSavedMethods = savedPaymentMethods.filter(
    (m) => m.providerSlug === paymentProviderSlug,
  );
  const usingSavedCard = savedMethodId !== "" &&
    providerSavedMethods.some((m) => m.id === savedMethodId);

  async function handlePlaceOrder() {
    // Card tokenization ran in the gateway plugins' own field components - with none
    // installed the order places tokenless and the provider settles it however it does
    if (paymentTokenRef.current) paymentTokenRef.current.value = "";
    formRef.current?.requestSubmit();
  }

  // After a successful add-address, select the new address for shipping (and billing if same-as-shipping is on) so the customer doesn't
  // have to hunt for it in the picker The result only reaches us as useActionState state, so an effect is the bridge
  useEffect(() => {
    if (addAddrState?.ok && "addressId" in addAddrState) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setShowNewAddress(false);
      if (addAddrState.addressId) {
        setShippingId(addAddrState.addressId);
        if (sameAsBilling || !billingId) {
          setBillingId(addAddrState.addressId);
        }
      }
    }
  }, [addAddrState, sameAsBilling, billingId]);

  const customerOk = email.trim().length > 0;
  // For the shipping step: logged-in customers need a picked address; guests need *something* typed into the country
  // field (their other fields get browser-level required validation at place-order time)
  const shippingAddressOk = isGuest ? !!shippingCountry : !!shippingId;
  const shippingMethodOk = shippingSel !== "";
  const shippingOk = shippingAddressOk && shippingMethodOk;
  const billingOk = sameAsBilling || (isGuest ? true : !!billingId);
  // A payment method must be selected before placing the order
  const paymentOk = paymentProviderSlug !== "";

  const selectedQuote = useMemo(
    () => shippingQuotes.find((q) => quoteKey(q) === shippingSel) ?? null,
    [shippingQuotes, shippingSel],
  );

  return (
    <div className="mt-10 grid gap-4">
      {placeState && !placeState.ok ? (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
        >
          {placeState.error}
        </p>
      ) : null}

      {scaMessage ? (
        <p
          role="status"
          className="glass-soft rounded-lg p-3 text-sm"
        >
          {scaMessage}
        </p>
      ) : null}

      {/* --- Step 1: Customer --- */}
      <StepCard
        number={1}
        title="Customer"
        state={stepState("customer")}
        summary={
          email ? (
            <span className="font-mono">{email}</span>
          ) : (
            <span>No email set</span>
          )
        }
        onEdit={isGuest ? () => goTo("customer") : undefined}
      >
        {isGuest ? (
          <div className="grid gap-3">
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-muted-foreground">
                Email <span className="text-destructive">*</span>
              </span>
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring"
              />
              <span className="text-xs text-muted-foreground">
                Order receipts and updates go here.{" "}
                <Link
                  href={`/account/login?returnTo=${encodeURIComponent("/checkout")}`}
                  className="underline underline-offset-2"
                >
                  Sign in
                </Link>{" "}
                to use a saved address.
              </span>
            </label>
            <ContinueButton
              onClick={() => {
                void setCartEmailAction(email);
                completeAndAdvance("customer", "shipping");
              }}
              disabled={!customerOk}
            />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            Signed in as <span className="font-mono">{email}</span>.
          </p>
        )}
      </StepCard>

      {/* --- Step 2: Shipping (address + method) --- */}
      <StepCard
        number={2}
        title="Shipping"
        state={stepState("shipping")}
        summary={
          <>
            {shippingAddress ? (
              <>
                {oneLine(shippingAddress)}
                {selectedQuote ? (
                  <span className="mt-0.5 block text-xs">
                    {selectedQuote.name} ·{" "}
                    {formatPrice(selectedQuote.priceCents, currency)}
                  </span>
                ) : null}
              </>
            ) : (
              <span>Ship-to {shippingCountry || "-"}</span>
            )}
          </>
        }
        onEdit={() => goTo("shipping")}
      >
        <div className="grid gap-6">
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Shipping address
            </h3>
            {isGuest ? (
              <InlineAddressFieldset
                slot="shipping"
                countries={countries}
                regionsByCountry={regionsByCountry}
                defaultCountry={countrySettings.defaultCountryCode}
                onCountryChange={setGuestShippingCountry}
                onRegionChange={setGuestShippingRegion}
                onPostalChange={setGuestShippingPostal}
              />
            ) : addresses.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">
                No saved addresses - add one below to continue.
              </p>
            ) : (
              <AddressPicker
                addresses={addresses}
                value={shippingId}
                onChange={(id) => {
                  setShippingId(id);
                  if (sameAsBilling) setBillingId(id);
                }}
              />
            )}

            {!isGuest && (
              <>
                <button
                  type="button"
                  onClick={() => setShowNewAddress((v) => !v)}
                  className="mt-3 text-xs text-foreground underline underline-offset-2"
                >
                  {showNewAddress ? "Cancel" : "+ Add a new address"}
                </button>

                {showNewAddress && (
                  <NewAddressForm
                    action={addAddrAction}
                    pending={addAddrPending}
                    error={
                      addAddrState && !addAddrState.ok
                        ? addAddrState.error
                        : null
                    }
                    countries={countries}
                    regionsByCountry={regionsByCountry}
                    defaultCountry={countrySettings.defaultCountryCode}
                  />
                )}
              </>
            )}
          </div>

          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Shipping method
            </h3>
            {!shippingCountry ? (
              <p className="mt-2 text-sm text-muted-foreground">
                Pick a shipping address to see available methods.
              </p>
            ) : shippingLoading ? (
              <p className="mt-2 text-sm text-muted-foreground">
                Loading quotes for {shippingCountry}…
              </p>
            ) : shippingQuotes.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">
                No shipping method available for {shippingCountry}.
              </p>
            ) : (
              <ul className="mt-3 space-y-2 text-sm">
                {orderedQuotes.map((q, i) => {
                  const groupLabel =
                    i === 0 || isPickupQuote(orderedQuotes[i - 1]) !== isPickupQuote(q)
                      ? isPickupQuote(q)
                        ? "Collect in store"
                        : "Deliver to me"
                      : null;
                  const loc = isPickupQuote(q) ? pickupBySlug.get(pickupSlug(q)) : undefined;
                  return (
                    <Fragment key={quoteKey(q)}>
                      {/* Only label the groups when both kinds are offered - one heading over one list is noise */}
                      {groupLabel && bothKindsOffered ? (
                        <li className="pt-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground first:pt-0">
                          {groupLabel}
                        </li>
                      ) : null}
                      {renderQuote(q, loc)}
                    </Fragment>
                  );
                })}
              </ul>
            )}
          </div>

          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={sameAsBilling}
              onChange={(e) => {
                setSameAsBilling(e.target.checked);
                if (e.target.checked) setBillingId(shippingId);
              }}
              className="size-4 rounded accent-primary"
            />
            Billing address is the same as shipping
          </label>

          <ContinueButton
            onClick={() =>
              completeAndAdvance(
                "shipping",
                sameAsBilling ? "review" : "billing",
              )
            }
            disabled={!shippingOk}
          />
        </div>
      </StepCard>

      {/* --- Step 3: Billing --- */}
      <StepCard
        number={3}
        title="Billing"
        state={stepState("billing")}
        summary={
          sameAsBilling ? (
            <span>Same as shipping</span>
          ) : billingAddress ? (
            oneLine(billingAddress)
          ) : (
            <span>-</span>
          )
        }
        onEdit={() => goTo("billing")}
      >
        <div className="grid gap-3">
          {sameAsBilling ? (
            <p className="text-sm text-muted-foreground">
              Using the shipping address above. Untick &ldquo;billing same as
              shipping&rdquo; on the previous step to set a different address.
            </p>
          ) : isGuest ? (
            <InlineAddressFieldset
              slot="billing"
              countries={countries}
              regionsByCountry={regionsByCountry}
              defaultCountry={countrySettings.defaultCountryCode}
            />
          ) : addresses.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No saved addresses yet - go back to the shipping step to add one.
            </p>
          ) : (
            <AddressPicker
              addresses={addresses}
              value={billingId}
              onChange={setBillingId}
            />
          )}
          <ContinueButton
            onClick={() => completeAndAdvance("billing", "review")}
            disabled={!billingOk}
          />
        </div>
      </StepCard>

      {/* --- Step 4: Review & place order --- */}
      <StepCard number={4} title="Review & place order" state={stepState("review")}>
        <div className="grid gap-4">
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Payment method
            </h3>
            {!shippingCountry ? (
              <p className="mt-2 text-sm text-muted-foreground">
                Pick a shipping address to see available payment methods.
              </p>
            ) : paymentLoading ? (
              <p className="mt-2 text-sm text-muted-foreground">
                Loading payment methods…
              </p>
            ) : paymentMethods.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">
                No payment method available for {shippingCountry}.
              </p>
            ) : (
              <ul className="mt-3 space-y-2 text-sm">
                {paymentMethods
                  .slice()
                  .sort((a, b) => a.sortOrder - b.sortOrder)
                  .map((m) => {
                    const checked = m.providerSlug === paymentProviderSlug;
                    return (
                      <li key={m.providerSlug}>
                        <label
                          className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 transition-colors ${
                            checked
                              ? "border-primary ring-1 ring-primary"
                              : "border-border hover:bg-muted"
                          }`}
                        >
                          <input
                            type="radio"
                            name="paymentMethodRadio"
                            value={m.providerSlug}
                            checked={checked}
                            onChange={() =>
                              setPickedProviderSlug(m.providerSlug)
                            }
                            className="size-4 accent-primary"
                          />
                          <span>{m.displayName}</span>
                        </label>
                      </li>
                    );
                  })}
              </ul>
            )}
            {providerSavedMethods.length > 0 && (
              <div className="mt-3 grid gap-2 text-sm">
                {providerSavedMethods.map((m) => (
                  <label
                    key={m.id}
                    className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 transition-colors ${
                      savedMethodId === m.id
                        ? "border-primary ring-1 ring-primary"
                        : "border-border hover:bg-muted"
                    }`}
                  >
                    <input
                      type="radio"
                      name="savedCardChoice"
                      checked={savedMethodId === m.id}
                      onChange={() => setSavedMethodId(m.id)}
                      className="size-4 accent-primary"
                    />
                    <span>
                      {m.brand || "Card"} •••• {m.last4}
                      {m.expMonth > 0 && (
                        <span className="text-muted-foreground">
                          {" "}
                          (exp {String(m.expMonth).padStart(2, "0")}/{m.expYear})
                        </span>
                      )}
                    </span>
                  </label>
                ))}
                <label
                  className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 transition-colors ${
                    savedMethodId === ""
                      ? "border-primary ring-1 ring-primary"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  <input
                    type="radio"
                    name="savedCardChoice"
                    checked={savedMethodId === ""}
                    onChange={() => setSavedMethodId("")}
                    className="size-4 accent-primary"
                  />
                  <span>Use a new card</span>
                </label>
              </div>
            )}
            {(() => {
              const instr = paymentMethods.find(
                (m) => m.providerSlug === paymentProviderSlug,
              )?.instructions;
              return instr ? (
                <p className="glass-soft mt-3 whitespace-pre-line rounded-lg p-3 text-sm text-muted-foreground">
                  {instr}
                </p>
              ) : null;
            })()}
          </div>

          <label className="flex flex-col gap-1 text-sm">
            <span className="text-muted-foreground">
              Order notes (optional)
            </span>
            <textarea
              value={customerNote}
              onChange={(e) => setCustomerNote(e.target.value)}
              rows={2}
              maxLength={500}
              className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring"
              placeholder="Delivery instructions, gift message, etc."
            />
          </label>

          <label className="flex flex-col gap-1 text-sm">
            <span className="text-muted-foreground">
              Gift card (optional)
            </span>
            <input
              type="text"
              value={giftCardCodes}
              onChange={(e) => setGiftCardCodes(e.target.value)}
              className="rounded-lg border border-input bg-card px-3 py-2 font-mono outline-none focus:border-ring"
              placeholder="GC-XXXX-XXXX-XXXX"
              autoComplete="off"
            />
            <span className="text-xs text-muted-foreground">
              Applied at checkout - covers part or all of your total; you pay
              any remaining balance. Separate multiple codes with commas.
            </span>
          </label>

          {storeCreditBalance > 0 && (
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                checked={useStoreCredit}
                onChange={(e) => setUseStoreCredit(e.target.checked)}
                className="mt-0.5 size-4 rounded accent-primary"
              />
              <span>
                Use my store credit (
                {formatPrice(storeCreditBalance, currency)} available)
                <span className="mt-0.5 block text-xs text-muted-foreground">
                  Applied after gift cards; any remainder is charged to your
                  payment method.
                </span>
              </span>
            </label>
          )}

          <label className="flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              checked={tosAccepted}
              onChange={(e) => setTosAccepted(e.target.checked)}
              className="mt-0.5 size-4 rounded accent-primary"
            />
            <span>
              I agree to the{" "}
              <a
                href="/terms"
                target="_blank"
                rel="noreferrer"
                className="underline underline-offset-2"
              >
                terms of service
              </a>{" "}
              and{" "}
              <a
                href="/privacy"
                target="_blank"
                rel="noreferrer"
                className="underline underline-offset-2"
              >
                privacy policy
              </a>
              .
            </span>
          </label>

          <button
            type="button"
            onClick={handlePlaceOrder}
            disabled={
              placePending ||
              !customerOk ||
              !shippingOk ||
              !billingOk ||
              !paymentOk ||
              !tosAccepted
            }
            className="inline-flex h-12 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {placePending ? "Placing order…" : "Place order"}
          </button>
        </div>
      </StepCard>

      {/* The order form sits at the end of the tree and carries every step's values via
          hidden / controlled inputs */}
      <form
        id="place-order-form"
        ref={formRef}
        action={placeAction}
        className="hidden"
      >
        <input type="hidden" name="email" value={email} />
        {/* Set imperatively by handlePlaceOrder after card tokenization. */}
        <input type="hidden" name="paymentToken" ref={paymentTokenRef} />
        <input
          type="hidden"
          name="customerNote"
          value={customerNote}
        />
        <input
          type="hidden"
          name="giftCardCodes"
          value={giftCardCodes}
        />
        <input
          type="hidden"
          name="useStoreCredit"
          value={useStoreCredit ? "1" : ""}
        />
        <input
          type="hidden"
          name="savedPaymentMethodId"
          value={usingSavedCard ? savedMethodId : ""}
        />
        <input
          type="hidden"
          name="shippingCarrierSlug"
          value={selectedQuote?.carrierSlug ?? ""}
        />
        <input
          type="hidden"
          name="shippingMethodCode"
          value={selectedQuote?.code ?? ""}
        />
        <input
          type="hidden"
          name="paymentProviderSlug"
          value={paymentProviderSlug}
        />
        <input
          type="hidden"
          name="billingSameAsShipping"
          value={sameAsBilling ? "1" : ""}
        />
        {!isGuest && (
          <>
            <input type="hidden" name="shippingAddressId" value={shippingId} />
            <input
              type="hidden"
              name="billingAddressId"
              value={sameAsBilling ? shippingId : billingId}
            />
          </>
        )}
      </form>
    </div>
  );
}

function StepCard({
  number,
  title,
  state,
  summary,
  onEdit,
  children,
}: {
  number: number;
  title: string;
  state: "active" | "completed" | "pending";
  summary?: ReactNode;
  onEdit?: () => void;
  children: ReactNode;
}) {
  // Body uses `hidden` rather than conditional rendering so the form inputs inside stay
  // mounted and submit with the rest of the order when a step is collapsed
  return (
    <section className="glass rounded-xl">
      <header className="flex items-center justify-between gap-3 px-5 py-4 sm:px-6">
        <div className="flex items-center gap-3 min-w-0">
          <span
            className={`grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold ${
              state === "completed"
                ? "bg-foreground text-background"
                : state === "active"
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted text-muted-foreground"
            }`}
            aria-hidden
          >
            {state === "completed" ? "✓" : number}
          </span>
          <h2 className="truncate text-base font-semibold">{title}</h2>
        </div>
        {state === "completed" && onEdit ? (
          <button
            type="button"
            onClick={onEdit}
            className="text-xs text-muted-foreground underline-offset-2 hover:underline"
          >
            Edit
          </button>
        ) : null}
      </header>
      <div
        hidden={state !== "active"}
        className="border-t border-border px-5 py-5 sm:px-6"
      >
        {children}
      </div>
      {state === "completed" && summary ? (
        <div className="border-t border-border px-5 py-3 text-sm text-muted-foreground sm:px-6">
          {summary}
        </div>
      ) : null}
    </section>
  );
}

function ContinueButton({
  onClick,
  disabled,
}: {
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="mt-2 inline-flex h-11 w-full items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50 sm:w-auto sm:self-end sm:px-8"
    >
      Continue
    </button>
  );
}

function oneLine(a: Address): ReactNode {
  return (
    <span>
      {a.firstName} {a.lastName} - {a.street1}, {a.city}{" "}
      {a.postalCode}, {a.countryCode}
    </span>
  );
}

function AddressPicker({
  addresses,
  value,
  onChange,
}: {
  addresses: Address[];
  value: string;
  onChange: (id: string) => void;
}) {
  return (
    <ul className="mt-3 grid gap-2 sm:grid-cols-2">
      {addresses.map((a) => {
        const checked = a.id === value;
        return (
          <li key={a.id}>
            <label
              className={`block cursor-pointer rounded-lg border p-3 text-sm leading-relaxed transition-colors ${
                checked
                  ? "border-primary ring-1 ring-primary"
                  : "border-border hover:bg-muted"
              }`}
            >
              <input
                type="radio"
                name={`address-picker-${onChange.name || "anon"}`}
                value={a.id}
                checked={checked}
                onChange={() => onChange(a.id)}
                className="sr-only"
              />
              <p className="font-medium">
                {a.firstName} {a.lastName}
              </p>
              {a.company ? (
                <p className="text-muted-foreground">{a.company}</p>
              ) : null}
              <p>{a.street1}</p>
              {a.street2 ? <p>{a.street2}</p> : null}
              <p>
                {a.city}
                {a.state ? `, ${a.state}` : ""} {a.postalCode}
              </p>
              <p>{a.countryCode}</p>
            </label>
          </li>
        );
      })}
    </ul>
  );
}

function Input({
  name,
  label,
  required = false,
  maxLength,
}: {
  name: string;
  label: string;
  required?: boolean;
  maxLength?: number;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">
        {label}
        {required ? <span className="ml-0.5 text-destructive">*</span> : null}
      </span>
      <input
        type="text"
        name={name}
        required={required}
        maxLength={maxLength}
        className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring"
      />
    </label>
  );
}
function InlineAddressFieldset({
  slot,
  countries,
  regionsByCountry,
  defaultCountry,
  onCountryChange,
  onRegionChange,
  onPostalChange,
}: {
  slot: "billing" | "shipping";
  countries: CountryOption[];
  regionsByCountry: Record<string, RegionOption[]>;
  defaultCountry?: string;
  onCountryChange?: (value: string) => void;
  onRegionChange?: (value: string) => void;
  onPostalChange?: (value: string) => void;
}) {
  const [country, setCountry] = useState(defaultCountry ?? "");
  const handleCountryChange = (value: string) => {
    setCountry(value);
    onCountryChange?.(value);
    onRegionChange?.("");
  };
  return (
    <div className="mt-3 grid gap-3 rounded-lg border border-border p-4">
      <div className="grid grid-cols-2 gap-3">
        <InlineInput slot={slot} field="firstName" label="First name" required />
        <InlineInput slot={slot} field="lastName" label="Last name" required />
      </div>
      <InlineInput slot={slot} field="company" label="Company (optional)" />
      <InlineInput slot={slot} field="street1" label="Street address" required />
      <InlineInput slot={slot} field="street2" label="Apt, suite, etc. (optional)" />
      <InlineInput slot={slot} field="city" label="City" required />
      <div className="grid grid-cols-2 gap-3">
        <InlineInput slot={slot} field="postalCode" label="Postal code" required onChange={onPostalChange} />
        <CountrySelect
          name={`${slot}.countryCode`}
          label="Country"
          countries={countries}
          defaultValue={defaultCountry}
          required
          onChange={handleCountryChange}
        />
      </div>
      {/* Countries with seeded subdivisions get a dropdown, everything else free text */}
      <StateField
        key={`${slot}-state-${country}`}
        name={`${slot}.state`}
        country={country}
        regions={regionsByCountry[country]}
        onChange={onRegionChange}
      />
      <InlineInput slot={slot} field="phone" label="Phone (optional)" />
    </div>
  );
}

// Owns its own country state so the state field can switch between dropdown and free
// text without leaking that to the parent, which just watches the action result
function NewAddressForm({
  action,
  pending,
  error,
  countries,
  regionsByCountry,
  defaultCountry,
}: {
  action: (formData: FormData) => void;
  pending: boolean;
  error: string | null;
  countries: CountryOption[];
  regionsByCountry: Record<string, RegionOption[]>;
  defaultCountry?: string;
}) {
  const [country, setCountry] = useState(defaultCountry ?? "");
  return (
    <form
      action={action}
      className="mt-3 grid gap-3 rounded-lg border border-border p-4"
    >
      <div className="grid grid-cols-2 gap-3">
        <Input name="firstName" label="First name" required />
        <Input name="lastName" label="Last name" required />
      </div>
      <Input name="company" label="Company (optional)" />
      <Input name="street1" label="Street address" required />
      <Input name="street2" label="Apt, suite, etc. (optional)" />
      <Input name="city" label="City" required />
      <div className="grid grid-cols-2 gap-3">
        <Input name="postalCode" label="Postal code" required />
        <CountrySelect
          name="countryCode"
          label="Country"
          countries={countries}
          defaultValue={defaultCountry}
          required
          formId={null}
          onChange={setCountry}
        />
      </div>
      <StateField
        key={`new-address-state-${country}`}
        name="state"
        country={country}
        regions={regionsByCountry[country]}
        formId={null}
      />
      <Input name="phone" label="Phone (optional)" />
      {error ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : null}
      <button
        type="submit"
        disabled={pending}
        className="mt-2 inline-flex h-9 items-center justify-center rounded-md border border-border text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
      >
        {pending ? "Saving…" : "Save address"}
      </button>
    </form>
  );
}
function StateField({
  name,
  country,
  regions,
  formId = "place-order-form",
  onChange,
}: {
  name: string;
  country: string;
  onChange?: (value: string) => void;
  // Backend-seeded subdivisions for this country; falls back to the static list when not provided (keeps the picker
  // working offline / for countries the channel hasn't fetched)
  regions?: RegionOption[];
  formId?: string | null;
}) {
  const subs = regions && regions.length > 0 ? regions : subdivisionsFor(country);
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">State / region</span>
      {subs ? (
        <select
          name={name}
          form={formId ?? undefined}
          defaultValue=""
          onChange={onChange ? (e) => onChange(e.target.value) : undefined}
          className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring"
        >
          <option value="" disabled>
            Select…
          </option>
          {subs.map((s) => (
            <option key={s.code} value={s.code}>
              {s.name}
            </option>
          ))}
        </select>
      ) : (
        <input
          type="text"
          name={name}
          form={formId ?? undefined}
          disabled={!country}
          onChange={onChange ? (e) => onChange(e.target.value) : undefined}
          placeholder={country ? "" : "Pick a country first"}
          className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring disabled:opacity-50"
        />
      )}
    </label>
  );
}

function CountrySelect({
  name,
  label,
  countries,
  required = false,
  formId = "place-order-form",
  defaultValue,
  onChange,
}: {
  name: string;
  label: string;
  countries: CountryOption[];
  required?: boolean;
  formId?: string | null;
  defaultValue?: string;
  onChange?: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">
        {label}
        {required ? <span className="ml-0.5 text-destructive">*</span> : null}
      </span>
      <select
        name={name}
        form={formId ?? undefined}
        required={required}
        defaultValue={defaultValue ?? ""}
        onChange={onChange ? (e) => onChange(e.target.value) : undefined}
        className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring"
      >
        <option value="" disabled>
          Select a country…
        </option>
        {countries.map((c) => (
          <option key={c.code} value={c.code}>
            {c.emojiFlag ? `${c.emojiFlag} ` : ""}
            {c.name} ({c.code})
          </option>
        ))}
      </select>
    </label>
  );
}

function InlineInput({
  slot,
  field,
  label,
  required = false,
  maxLength,
  onChange,
  uppercase = false,
}: {
  slot: "billing" | "shipping";
  field: string;
  label: string;
  required?: boolean;
  maxLength?: number;
  onChange?: (value: string) => void;
  uppercase?: boolean;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-muted-foreground">
        {label}
        {required ? <span className="ml-0.5 text-destructive">*</span> : null}
      </span>
      <input
        type="text"
        name={`${slot}.${field}`}
        form="place-order-form"
        required={required}
        maxLength={maxLength}
        onChange={
          onChange
            ? (e) =>
                onChange(uppercase ? e.target.value.toUpperCase() : e.target.value)
            : undefined
        }
        className={`rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring ${
          uppercase ? "uppercase" : ""
        }`}
      />
    </label>
  );
}
