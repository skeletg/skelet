import type { NextRequest } from "next/server";

import { completeSso } from "@skelet/storefront-next/auth/sso";

// Where the store's sign-in provider sends the shopper back
export async function GET(req: NextRequest) {
  const sp = req.nextUrl.searchParams;
  return completeSso({ code: sp.get("code"), state: sp.get("state"), error: sp.get("error") });
}
