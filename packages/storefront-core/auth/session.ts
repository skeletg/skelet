// Cookie-backed customer session - httpOnly access + refresh tokens, so client JS can't lift them. Read from server
// components and actions only; gqlFetch sends the access token as Authorization: Bearer


import { request } from "../request";
import {
  ACCESS_TOKEN_COOKIE,
  ACCESS_TOKEN_MAX_AGE,
  MASQUERADE_COOKIE,
  REFRESH_TOKEN_COOKIE,
  REFRESH_TOKEN_MAX_AGE,
  sessionCookieOptions,
} from "./cookies";

// Re-exported for the reads and actions that import them from here
export { ACCESS_TOKEN_COOKIE, REFRESH_TOKEN_COOKIE, MASQUERADE_COOKIE };

export async function getAccessToken(): Promise<string | null> {
  const store = await request().cookies();
  return store.get(ACCESS_TOKEN_COOKIE)?.value ?? null;
}

export async function getRefreshToken(): Promise<string | null> {
  const store = await request().cookies();
  return store.get(REFRESH_TOKEN_COOKIE)?.value ?? null;
}

export async function setSessionCookies(
  accessToken: string,
  refreshToken: string,
): Promise<void> {
  const store = await request().cookies();
  const opts = sessionCookieOptions();
  // Access token is short-lived (15 min in the seed config) but we give the cookie a longer life so the browser keeps presenting it until
  // the proxy proactively refreshes - the backend will reject any stale JWT regardless of cookie age
  store.set(ACCESS_TOKEN_COOKIE, accessToken, {
    ...opts,
    maxAge: ACCESS_TOKEN_MAX_AGE,
  });
  store.set(REFRESH_TOKEN_COOKIE, refreshToken, {
    ...opts,
    maxAge: REFRESH_TOKEN_MAX_AGE,
  });
}

export async function clearSessionCookies(): Promise<void> {
  const store = await request().cookies();
  store.delete(ACCESS_TOKEN_COOKIE);
  store.delete(REFRESH_TOKEN_COOKIE);
}
