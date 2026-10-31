import { render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { BEACON } from "@skelet/storefront-next/beacons/paths";

let pathname = "/";
vi.mock("next/navigation", () => ({ usePathname: () => pathname }));

const snapshot = vi.fn(() => ({ type: 0, childNodes: [] }));
vi.mock("rrweb-snapshot", () => ({ snapshot }));
vi.mock("web-vitals", () => ({ onLCP: () => {}, onCLS: () => {}, onINP: () => {}, onTTFB: () => {} }));

const beaconFetch = vi.fn<(path: string, opts: { body?: string }) => Promise<Response>>(async (path) =>
  path === BEACON.config ? new Response(JSON.stringify({ enabled: true, samplingRate: 100 })) : new Response(null),
);
vi.mock("@skelet/storefront-next/beacons/client", () => ({
  beaconFetch: (p: string, o: { body?: string }) => beaconFetch(p, o),
}));

const { LensTracker } = await import("@/app/components/LensTracker");

// What the tracker sent to the collect endpoint, by event type
function sentTypes(): string[] {
  return beaconFetch.mock.calls
    .filter(([path]) => path === BEACON.collect)
    .flatMap(([, o]) => (JSON.parse(o.body ?? "{}") as { events: { type: string }[] }).events.map((e) => e.type));
}

async function visit(path: string) {
  pathname = path;
  window.history.replaceState(null, "", path);
  const { unmount } = render(<LensTracker enabled channelId="ch" />);
  await waitFor(() => expect(beaconFetch).toHaveBeenCalledWith(BEACON.config, expect.anything()));
  await vi.dynamicImportSettled();
  unmount(); // flushes the queue
}

describe("LensTracker", () => {
  beforeEach(() => {
    snapshot.mockClear();
    beaconFetch.mockClear();
  });

  it("counts a private page view without capturing its DOM", async () => {
    await visit("/de/account/orders");
    expect(sentTypes()).toEqual(["page_view"]);
    expect(snapshot).not.toHaveBeenCalled();
  });

  it("still snapshots public pages for the heatmap", async () => {
    await visit("/products/shirt");
    expect(sentTypes()).toEqual(["page_view", "snapshot"]);
  });
});
