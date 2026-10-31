import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/app/test/render";
import type { CheckoutState, DeliveryChoice, DeliveryPointsResult } from "@/app/lib/checkout/read";

const actions = vi.hoisted(() => ({ saveGroupDeliveryAction: vi.fn(async () => ({})) }));
vi.mock("@/app/lib/checkout/actions", () => ({
  ...actions,
  saveDestinationStep: vi.fn(async () => ({})),
  saveDeliveryPointAction: vi.fn(async () => ({})),
  saveSplitDeliveryAction: vi.fn(async () => ({})),
}));
vi.mock("@/app/components/LocaleLink", () => ({ Link: ({ href, children, ...props }: React.ComponentProps<"a">) => <a href={href} {...props}>{children}</a> }));
const { DeliveryStep, fastestChoice } = await import("./delivery-step");

beforeEach(() => actions.saveGroupDeliveryAction.mockReset());
const point = { id: "point-21", name: "Central station", source: "manual-points", street1: "1 Main Street", street2: "", city: "Belgrade", region: "", postalCode: "11000", countryCode: "RS", distanceKm: null, phone: null, hours: "9–5", instructions: "" };
const choice = { optionId: "option-1", optionName: "Pickup point", kind: "parcel_locker", kindLabel: "Pickup point", shipTo: "POINT", pointLabel: "Pickup point", carrierSlug: "builtin", methodCode: "standard", methodName: "Standard", description: "", priceCents: 3000, estimatedDays: 2 };
function renderStep({ chosen = true, result = { points: [point], unavailable: false }, groups = 1 }: { chosen?: boolean; result?: DeliveryPointsResult; groups?: number } = {}) {
  const checkout = {
    destination: { countryCode: "RS", postalCode: "11000", regionCode: null },
    deliveryGroups: Array.from({ length: groups }, (_, i) => ({ key: `group-${i}`, lineIds: [`line-${i}`], choices: [choice], selection: chosen ? { ...choice, point } : null, shipments: [] })),
    requirements: Array.from({ length: groups }, (_, i) => ({ scope: `group-${i}`, type: chosen ? "point" : "delivery_option", mode: "REQUIRED", status: chosen ? "SATISFIED" : "MISSING", label: "Pickup location", code: "", message: "" })),
    digitalLineIds: [], canSplit: false, splitDelivery: false,
  } as unknown as CheckoutState;
  if (chosen) checkout.requirements.push({ scope: "group-0", type: "collector", mode: "REQUIRED", status: "MISSING", label: "Who collects", code: "", message: "" });
  return renderWithIntl(<DeliveryStep checkout={checkout} suggested={{ countryCode: "RS", postalCode: "" }} countries={[{ code: "RS", name: "Serbia", emojiFlag: "" }]} lines={{}} arrivals={{}} points={Object.fromEntries(checkout.deliveryGroups.map((g) => [g.key, result]))} pointSearch={{ group: "group-0", query: "station" }} thresholds={{ show: true, subtotalCents: 15000, minOrderCents: 0, freeShippingCents: 10000 }} notice={false} currency="EUR" />);
}

it("requires explicit Continue even when all delivery requirements are satisfied", () => {
  renderStep();
  expect(screen.getByRole("link", { name: "Continue to details" })).toHaveAttribute("href", "/checkout?step=details");
});

it("keeps Continue visible and disabled until a choice is saved", () => {
  renderStep({ chosen: false });
  expect(screen.getByRole("button", { name: "Continue to details" })).toBeDisabled();
});

it("offers server-side search and keeps the selected point visible outside the results", () => {
  renderStep({ result: { points: [], unavailable: false } });
  expect(screen.getByText("Central station")).toBeVisible();
  const search = screen.getByRole("button", { name: "Search" }).closest("form")!;
  expect(search.method).toBe("get");
  expect(new FormData(search).get("pointGroup")).toBe("group-0");
  expect(new FormData(search).get("pointSearch")).toBe("station");
  expect(new FormData(search).get("step")).toBe("delivery");
});

it("distinguishes source failure from an empty search and offers Retry", () => {
  renderStep({ result: { points: [], unavailable: true } });
  expect(screen.getByRole("alert")).toHaveTextContent("Pickup locations could not be loaded");
  expect(screen.getByRole("button", { name: "Retry" })).toBeVisible();
  expect(screen.queryByText(/No pickup locations match/)).not.toBeInTheDocument();
});

it("explains automatic groups and totals paid delivery even above a threshold", () => {
  renderStep({ groups: 2 });
  expect(screen.getByText(/These items need different delivery options/)).toBeVisible();
  expect(screen.getByText(/Product-specific charges may still apply/)).toBeVisible();
  expect(screen.queryByText(/free shipping unlocked/i)).not.toBeInTheDocument();
  expect(screen.getByText("Selected delivery total").parentElement).toHaveTextContent("60.00");
});

it("prevents overlapping changes while a delivery selection is saving", async () => {
  let finish!: () => void;
  actions.saveGroupDeliveryAction.mockImplementationOnce(() => new Promise((resolve) => { finish = () => resolve({}); }));
  renderStep({ chosen: false, groups: 2 });
  const choices = screen.getAllByRole("radio");
  fireEvent.click(choices[0]);
  await waitFor(() => expect(choices[1]).toBeDisabled());
  expect(screen.getByRole("status")).toHaveTextContent("Saving your delivery choice");
  finish();
  await waitFor(() => expect(choices[1]).not.toBeDisabled());
});

// Door delivery from one option: the store's Standard, Express (weekends too) and a free Economy, with what the server
// formatted for each arrival
const door = (methodCode: string, methodName: string, priceCents: number, estimatedDays: number, estimatedArrival: string | null, more: Partial<typeof choice> & { carrierName?: string } = {}) => ({
  ...choice, optionId: "door", optionName: "Delivery", kind: "ship_to_address", kindLabel: "Ship to address", shipTo: "CUSTOMER", pointLabel: null,
  carrierSlug: "builtin", carrierName: "Store rates", methodCode, methodName, priceCents, estimatedDays, estimatedArrival, ...more,
});
type Door = ReturnType<typeof door>;
const keyOf = (c: Door) => `${c.optionId}::${c.carrierSlug}::${c.methodCode}`;

function renderDoor(choices: Door[], arrivals: Record<string, string> = {}) {
  const checkout = {
    destination: { countryCode: "RS", postalCode: "11000", regionCode: null },
    deliveryGroups: [{ key: "group-0", lineIds: ["line-0"], choices, selection: null, shipments: [] }],
    requirements: [{ scope: "group-0", type: "delivery_option", mode: "REQUIRED", status: "MISSING", label: "Delivery", code: "", message: "" }],
    digitalLineIds: [], canSplit: false, splitDelivery: false,
  } as unknown as CheckoutState;
  return renderWithIntl(<DeliveryStep checkout={checkout} suggested={{ countryCode: "RS", postalCode: "" }} countries={[{ code: "RS", name: "Serbia", emojiFlag: "" }]} lines={{}} arrivals={arrivals} points={{}} pointSearch={{ group: "", query: "" }} thresholds={{ show: false, subtotalCents: 2000, minOrderCents: 0, freeShippingCents: 0 }} notice={false} currency="EUR" />);
}

const standard = door("standard", "Standard", 500, 4, "2026-10-08");
const express = door("express", "Express", 1200, 1, "2026-10-03");
const economy = door("economy", "Economy", 0, 5, "2026-10-09");

it("lists every method of a door option with its price, Free for nothing, and when it arrives", () => {
  renderDoor([standard, express, economy], { [keyOf(standard)]: "Thu 8 Oct", [keyOf(express)]: "Sat 3 Oct", [keyOf(economy)]: "Fri 9 Oct" });
  const radios = screen.getAllByRole("radio");
  expect(radios).toHaveLength(3);
  // Cheapest first
  expect(radios.map((r) => r.closest("label")?.textContent)).toEqual([
    expect.stringContaining("Economy"),
    expect.stringContaining("Standard"),
    expect.stringContaining("Express"),
  ]);
  const card = (name: RegExp) => screen.getByRole("radio", { name }).closest("label")!;
  expect(card(/Express/)).toHaveTextContent("Delivery · Express");
  expect(card(/Express/)).toHaveTextContent("12.00");
  expect(card(/Express/)).toHaveTextContent("Arrives Sat 3 Oct");
  expect(card(/Economy/)).toHaveTextContent("Free");
  expect(card(/Standard/)).toHaveTextContent("Arrives Thu 8 Oct");
});

it("marks the soonest arrival Fastest", () => {
  renderDoor([standard, express, economy]);
  expect(screen.getAllByText("Fastest")).toHaveLength(1);
  expect(screen.getByRole("radio", { name: /Express/ }).closest("label")).toHaveTextContent("Fastest");
});

it("shows no arrival line for a method the carrier gives no estimate", () => {
  renderDoor([door("standard", "Standard", 500, 0, null), express], { [keyOf(express)]: "Sat 3 Oct" });
  expect(screen.getByRole("radio", { name: /Standard/ }).closest("label")).not.toHaveTextContent("Arrives");
});

it("names the carrier when one option's methods come from several", () => {
  const dhl = door("standard", "Standard", 650, 2, "2026-10-06", { carrierSlug: "dhl", carrierName: "DHL" });
  renderDoor([standard, dhl]);
  const labels = screen.getAllByRole("radio").map((r) => r.closest("label")?.textContent ?? "");
  expect(labels.some((l) => l.includes("Delivery · Store rates · Standard"))).toBe(true);
  expect(labels.some((l) => l.includes("Delivery · DHL · Standard"))).toBe(true);
});

it("keeps two options of one kind apart by their names", () => {
  const lane = door("express", "Express", 1200, 1, "2026-10-03", { optionId: "lane", optionName: "Express lane" });
  renderDoor([standard, express, lane]);
  expect(screen.getAllByRole("radio")).toHaveLength(3);
  expect(screen.getByRole("radio", { name: /Express lane/ })).toHaveAttribute("value", "lane::builtin::express");
  expect(screen.getByRole("radio", { name: /Delivery\s*· Express/ })).toHaveAttribute("value", "door::builtin::express");
});

it("saves the chosen option, carrier and method together", async () => {
  const lane = door("express", "Express", 1200, 1, "2026-10-03", { optionId: "lane", optionName: "Express lane" });
  renderDoor([standard, lane]);
  fireEvent.click(screen.getByRole("radio", { name: /Express lane/ }));
  await waitFor(() => expect(actions.saveGroupDeliveryAction).toHaveBeenCalled());
  const form = (actions.saveGroupDeliveryAction.mock.calls[0] as unknown[]).find((a): a is FormData => a instanceof FormData);
  expect(form?.get("selection")).toBe("lane::builtin::express");
  expect(form?.get("groupKey")).toBe("group-0");
});

describe("fastestChoice", () => {
  const fri = (methodCode: string, estimatedDays: number, estimatedArrival: string | null, shipTo = "CUSTOMER") =>
    ({ ...door(methodCode, methodCode, 100, estimatedDays, estimatedArrival), shipTo }) as unknown as DeliveryChoice;

  it("goes by arrival date, so a 2-day weekend courier beats a 1-day weekday one from a Friday", () => {
    expect(fastestChoice([fri("weekday", 1, "2026-10-05"), fri("weekend", 2, "2026-10-04")])).toBe("door::builtin::weekend");
  });
  it("marks none on a tie", () => {
    expect(fastestChoice([fri("a", 1, "2026-10-05"), fri("b", 2, "2026-10-05")])).toBeNull();
  });
  it("ignores pickups and choices without a date", () => {
    expect(fastestChoice([fri("a", 1, "2026-10-05"), fri("pickup", 0, "2026-10-03", "POINT"), fri("none", 0, null)])).toBeNull();
  });
});
