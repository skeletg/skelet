// Sales-rep masquerade landing: installs a short-lived customer access token (minted by the admin masqueradeCustomer
// mutation) as the session cookie and bounces to the homepage

import { NextResponse, type NextRequest } from "next/server";

import {
  ACCESS_TOKEN_COOKIE,
  MASQUERADE_COOKIE,
  isMasqueradeToken,
  sessionCookieOptions,
} from "@skelet/storefront-core/auth/cookies";

const MASQUERADE_MAX_AGE = 15 * 60;

export function GET(req: NextRequest) {
  const res = new NextResponse(null, { status: 303, headers: { Location: "/", "Cache-Control": "no-store" } });
  const token = req.nextUrl.searchParams.get("token") ?? "";
  // Only a masquerade the backend minted - a customer's own token in a link would sign the clicker into the sender's account
  if (!isMasqueradeToken(token)) return res;
  const opts = { ...sessionCookieOptions(), maxAge: MASQUERADE_MAX_AGE };
  res.cookies.set(ACCESS_TOKEN_COOKIE, token, opts);
  res.cookies.set(MASQUERADE_COOKIE, "1", { ...opts, httpOnly: false });
  return res;
}
