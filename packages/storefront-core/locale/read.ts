import { request } from "../request";
import { getChannelLocales, type ChannelLocales } from "./backend-locales";

// From the route's locale segment, never from a cookie. A cookie can't be read on the routes that serve most of the
// site (they render without a request), so the segment is part of the cache key instead, one cached page per language
export async function getCurrentLocale(): Promise<string> {
  return request().locale();
}

// The channel the request is for - the route's channel segment, else the proxy's header, else the pinned one
export async function getCurrentChannelId(): Promise<string> {
  return request().channelId();
}

export async function getCurrentChannelLocales(): Promise<ChannelLocales> {
  return getChannelLocales(await getCurrentChannelId());
}
