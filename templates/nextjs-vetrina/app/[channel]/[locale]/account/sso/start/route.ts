import type { NextRequest } from "next/server";

import { startSso } from "@skelet/storefront-next/auth/sso";

// Where the sign-in button links - a GET, so it works before the page's scripts load
export async function GET(req: NextRequest) {
  return startSso({ returnTo: req.nextUrl.searchParams.get("returnTo") });
}
