import { afterEach, describe, expect, it, vi } from "vitest";

import { cachedFetch, invalidateData } from "./data-cache";

const init = (channel: string, body = '{"query":"{ x }"}'): RequestInit => ({ method: "POST", headers: { "X-Channel-Id": channel }, body });

describe("cachedFetch", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    invalidateData(["t"]);
  });

  it("answers a repeated read from the cache until its tag is invalidated", async () => {
    const fetchMock = vi.fn(async () => new Response('{"data":1}', { headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const read = () => cachedFetch("http://b/g", init("ch-1"), { tags: ["t"] }).then((r) => r.text());
    expect(await read()).toBe('{"data":1}');
    expect(await read()).toBe('{"data":1}');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(invalidateData(["t"])).toBe(1);
    await read();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("keeps channels apart, and a failed read out", async () => {
    const fetchMock = vi.fn(async () => new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    await cachedFetch("http://b/g", init("ch-1"), { tags: ["t"] });
    await cachedFetch("http://b/g", init("ch-2"), { tags: ["t"] });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    fetchMock.mockResolvedValue(new Response("down", { status: 502 }));
    await cachedFetch("http://b/g", init("ch-3"), { tags: ["t"] });
    await cachedFetch("http://b/g", init("ch-3"), { tags: ["t"] });
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("lets a read expire after the hint's seconds", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn(async () => new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    await cachedFetch("http://b/g", init("ch-9"), { tags: ["t"], revalidate: 60 });
    vi.advanceTimersByTime(61_000);
    await cachedFetch("http://b/g", init("ch-9"), { tags: ["t"], revalidate: 60 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });
});
