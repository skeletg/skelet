// Customer-session cookie names, lifetimes, and options - the single source of truth shared by the server-action helpers in auth.ts (which
// write via next/headers `cookies()`) and the Edge middleware (which writes on the NextResponse)

export const ACCESS_TOKEN_COOKIE = "storefront_access_token";
// Non-httpOnly marker so the banner can render server-side without decoding the token - carries no
// authority of its own, the masquerade ends when the session cookie does
export const MASQUERADE_COOKIE = "storefront_masquerade";
export const REFRESH_TOKEN_COOKIE = "storefront_refresh_token";

// Access-token JWT lives ~15 min server-side; the cookie outlives it so the browser
// keeps presenting it until the middleware proactively rotates
export const ACCESS_TOKEN_MAX_AGE = 60 * 60; // 1 hour
export const REFRESH_TOKEN_MAX_AGE = 60 * 60 * 24 * 30; // 30 days

// Base cookie options (path/httpOnly/sameSite/secure) both writers must agree on - a
// mismatch would create a second, shadowing cookie
export function sessionCookieOptions() {
  return {
    path: "/",
    httpOnly: true,
    sameSite: "lax" as const,
    secure: process.env.NODE_ENV === "production",
  };
}
export function decodeJwtExp(token: string): number | null {
  const parts = token.split(".");
  if (parts.length < 2) return null;
  try {
    const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const claims = JSON.parse(atob(b64)) as { exp?: unknown };
    return typeof claims.exp === "number" ? claims.exp : null;
  } catch {
    return null;
  }
}

// isAccessTokenExpiringSoon returns true when the access token is missing, unreadable, or within `skewSeconds` of expiry - i.e. when the
// middleware should refresh ahead of time (so a lost prefetch race isn't fatal: the old token is still valid until the real expiry)
export function isAccessTokenExpiringSoon(
  token: string | undefined | null,
  skewSeconds = 120,
): boolean {
  if (!token) return true;
  const exp = decodeJwtExp(token);
  if (exp == null) return true;
  return exp - Math.floor(Date.now() / 1000) < skewSeconds;
}
