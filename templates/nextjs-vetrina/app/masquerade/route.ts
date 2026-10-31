// Sales-rep masquerade landing: installs a short-lived customer access token (minted by the admin masqueradeCustomer
// mutation) as the session cookie and bounces to the homepage

import { NextResponse, type NextRequest } from "next/server";

import { ACCESS_TOKEN_COOKIE, MASQUERADE_COOKIE } from "@skelet/storefront-next/auth/session";

export async function GET(req: NextRequest) {
  const token = req.nextUrl.searchParams.get("token") ?? "";
  // req.url carries the container bind address (0.0.0.0) - build the redirect from the
  // Host header so cookies and destination share an origin
  const host =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? req.nextUrl.host;
  const proto = req.headers.get("x-forwarded-proto") ?? "http";
  const home = new URL(`${proto}://${host}/`);
  if (!token) return NextResponse.redirect(home);
  const res = NextResponse.redirect(home);
  const opts = {
    path: "/",
    sameSite: "lax" as const,
    secure: process.env.NODE_ENV === "production",
    maxAge: 15 * 60,
  };
  res.cookies.set(ACCESS_TOKEN_COOKIE, token, { ...opts, httpOnly: true });
  res.cookies.set(MASQUERADE_COOKIE, "1", opts);
  return res;
}
