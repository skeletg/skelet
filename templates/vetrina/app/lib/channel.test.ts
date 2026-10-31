import { describe, expect, it } from "vitest";

import { BUILD_PROBE_CHANNEL, channelForHost, hostOf, isChannelId, parseChannelHosts } from "@/app/lib/channel";
import { channelRoutePaths, internalPath, isPagePath } from "@/app/lib/channel-routing";

const US = "9ead3bed-c091-4757-bd3c-ab6bad95f353";
const EU = "03b1b50b-ae60-4eab-af0a-e858ba408e8c";

describe("host to channel", () => {
  const hosts = parseChannelHosts(`shop.example.com=${US}, EU.shop.example.com:443=${EU}, bad=not-a-uuid, =${US}`);

  it("parses only host=uuid pairs", () => {
    expect([...hosts.entries()]).toEqual([
      ["shop.example.com", US],
      ["eu.shop.example.com", EU],
    ]);
  });

  it("serves each mapped host its own channel", () => {
    expect(channelForHost("shop.example.com", hosts, "")).toBe(US);
    expect(channelForHost("eu.shop.example.com:8443", hosts, "")).toBe(EU);
    expect(channelForHost("eu.shop.example.com", hosts, US)).toBe(EU);
  });

  it("answers an unmapped host with the pin, or with nothing", () => {
    expect(channelForHost("other.example.com", hosts, US)).toBe(US);
    expect(channelForHost("other.example.com", hosts, "")).toBe("");
  });

  it("normalises hosts", () => {
    expect(hostOf("Shop.Example.com:3000")).toBe("shop.example.com");
    expect(hostOf("[::1]:3000")).toBe("[::1]");
    expect(isChannelId(US)).toBe(true);
    expect(isChannelId("x")).toBe(false);
    expect(isChannelId(BUILD_PROBE_CHANNEL)).toBe(false);
  });
});

describe("browser path to app route", () => {
  const codes = ["en", "de"];

  it("puts every page under its channel and locale", () => {
    expect(internalPath("/", US, codes, "en")).toBe(`/${US}/en`);
    expect(internalPath("/shoes", US, codes, "en")).toBe(`/${US}/en/shoes`);
    expect(internalPath("/de/shoes", US, codes, "en")).toBe(`/${US}/de/shoes`);
  });

  it("never lets a browser path name another channel", () => {
    expect(internalPath(`/${EU}/en/shoes`, US, codes, "en")).toBe(`/${US}/en/${EU}/en/shoes`);
  });

  it("leaves route handlers and files alone", () => {
    expect(internalPath("/api/revalidate", US, codes, "en")).toBeNull();
    expect(internalPath("/sitemap.xml", US, codes, "en")).toBeNull();
    expect(internalPath("/products/sitemap/0.xml", US, codes, "en")).toBeNull();
    expect(isPagePath("/coupon/SAVE")).toBe(false);
  });

  it("purges a storefront path in every locale of the channel", () => {
    expect(channelRoutePaths("/shoes", US, codes)).toEqual([`/${US}/en/shoes`, `/${US}/de/shoes`]);
    expect(channelRoutePaths("/", US, codes)).toEqual([`/${US}/en`, `/${US}/de`]);
    expect(channelRoutePaths("/de/shoes", US, codes)).toEqual([`/${US}/de/shoes`]);
  });
});
