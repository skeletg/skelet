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
    const fetchMock = vi.fn(async () => new Response('{"data":{}}'));
    vi.stubGlobal("fetch", fetchMock);
    await cachedFetch("http://b/g", init("ch-1"), { tags: ["t"] });
    await cachedFetch("http://b/g", init("ch-2"), { tags: ["t"] });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    fetchMock.mockImplementation(async () => new Response("down", { status: 502 }));
    await cachedFetch("http://b/g", init("ch-3"), { tags: ["t"] });
    await cachedFetch("http://b/g", init("ch-3"), { tags: ["t"] });
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("lets a read expire after the hint's seconds", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn(async () => new Response('{"data":{}}'));
    vi.stubGlobal("fetch", fetchMock);
    await cachedFetch("http://b/g", init("ch-9"), { tags: ["t"], revalidate: 60 });
    vi.advanceTimersByTime(61_000);
    await cachedFetch("http://b/g", init("ch-9"), { tags: ["t"], revalidate: 60 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });

  it("keeps no answer that carries errors or no data, and none the hint says not to keep", async () => {
    const fetchMock = vi.fn(async () => new Response('{"data":null,"errors":[{"message":"x"}]}'));
    vi.stubGlobal("fetch", fetchMock);
    await cachedFetch("http://b/g", init("ch-e"), { tags: ["t"] });
    await cachedFetch("http://b/g", init("ch-e"), { tags: ["t"] });
    fetchMock.mockImplementation(async () => new Response('{"data":{}}'));
    await cachedFetch("http://b/g", init("ch-z"), { tags: ["t"], revalidate: 0 });
    await cachedFetch("http://b/g", init("ch-z"), { tags: ["t"], revalidate: 0 });
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });

  it("fetches a miss once however many reads wait on it", async () => {
    let release!: () => void;
    const fetchMock = vi.fn(() => new Promise<Response>((r) => (release = () => r(new Response('{"data":1}')))));
    vi.stubGlobal("fetch", fetchMock);
    const reads = Array.from({ length: 5 }, () => cachedFetch("http://b/g", init("ch-c"), { tags: ["t"] }).then((r) => r.text()));
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    release();
    expect(await Promise.all(reads)).toEqual(Array(5).fill('{"data":1}'));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("doesn't store what a fetch got when a purge landed while it ran", async () => {
    let release!: () => void;
    const fetchMock = vi.fn(() => new Promise<Response>((r) => (release = () => r(new Response('{"data":"old"}')))));
    vi.stubGlobal("fetch", fetchMock);
    const read = cachedFetch("http://b/g", init("ch-p"), { tags: ["t"] });
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    invalidateData(["t"]);
    release();
    await read;
    fetchMock.mockImplementation(async () => new Response('{"data":"new"}'));
    expect(await (await cachedFetch("http://b/g", init("ch-p"), { tags: ["t"] })).text()).toBe('{"data":"new"}');
  });

  it("evicts the read used longest ago when it's full", async () => {
    const fetchMock = vi.fn(async () => new Response('{"data":{}}'));
    vi.stubGlobal("fetch", fetchMock);
    const read = (i: number) => cachedFetch("http://b/g", init(`lru-${i}`), { tags: ["t"] });
    for (let i = 0; i < 500; i++) await read(i);
    await read(0);
    await read(500);
    const before = fetchMock.mock.calls.length;
    await read(0);
    expect(fetchMock.mock.calls.length).toBe(before);
    await read(1);
    expect(fetchMock.mock.calls.length).toBe(before + 1);
  });
});
