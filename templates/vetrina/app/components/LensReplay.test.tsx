import { render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { BEACON } from "@/app/lib/beacons/paths";

let pathname = "/";
vi.mock("next/navigation", () => ({ usePathname: () => pathname }));

const stop = vi.fn();
const record = vi.fn((opts: { emit: (e: unknown) => void }) => {
  opts.emit({ type: 4, data: { href: location.href } }); // the Meta event every fresh recording opens with
  return stop;
});
vi.mock("rrweb", () => ({ record }));

const beaconFetch = vi.fn<(path: string, opts: unknown) => Promise<Response>>(async (path) =>
  path === BEACON.config
    ? new Response(JSON.stringify({ replayEnabled: true, replaySamplingRate: 100 }))
    : new Response(null),
);
vi.mock("@/app/lib/beacons/client", () => ({ beaconFetch: (p: string, o: unknown) => beaconFetch(p, o) }));

const { LensReplay } = await import("@/app/components/LensReplay");

const view = () => <LensReplay enabled channelId="ch" customerId="cust" />;

describe("LensReplay", () => {
  beforeEach(() => {
    sessionStorage.clear();
    record.mockClear();
    stop.mockClear();
    beaconFetch.mockClear();
  });

  it("pauses on a private page and resumes on the next public one with a fresh recording", async () => {
    pathname = "/products/shirt";
    const { rerender, unmount } = render(view());
    await waitFor(() => expect(record).toHaveBeenCalledTimes(1));

    pathname = "/de/checkout";
    rerender(view());
    expect(stop).toHaveBeenCalledTimes(1);
    expect(record).toHaveBeenCalledTimes(1);

    pathname = "/de/account/orders";
    rerender(view());
    expect(record).toHaveBeenCalledTimes(1);

    pathname = "/de/products/shirt";
    rerender(view());
    expect(record).toHaveBeenCalledTimes(2);
    unmount();
  });

  it("never starts while the session lands on a private page", async () => {
    pathname = "/account";
    const { rerender, unmount } = render(view());
    await waitFor(() => expect(beaconFetch).toHaveBeenCalledWith(BEACON.config, expect.anything()));
    await vi.dynamicImportSettled(); // rrweb loaded, so a start would have happened by now
    expect(record).not.toHaveBeenCalled();

    pathname = "/";
    rerender(view());
    await waitFor(() => expect(record).toHaveBeenCalledTimes(1));
    unmount();
  });
});
