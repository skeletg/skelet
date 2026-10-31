// GDPR self-serve export: streams the front `myDataExport` JSON bundle (profile +
// addresses + orders) as a download

import { redirect } from "next/navigation";
import { getLocale } from "next-intl/server";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import { getAccessToken } from "@skelet/storefront-next/auth/session";
import { MyDataExportDocument } from "@skelet/storefront-next/customer/operations.gql";
import { gqlFetch } from "@skelet/storefront-next/graphql";

export async function GET() {
  if (!(await getAccessToken())) {
    redirect(await withLocalePrefix("/account/login?returnTo=/account/privacy", await getLocale()));
  }
  let json: string;
  try {
    const res = await gqlFetch(MyDataExportDocument);
    json = res.data.myDataExport;
  } catch {
    // Token expired mid-flight or the backend refused - treat both as "sign in again"
    json = "";
  }
  if (!json) redirect(await withLocalePrefix("/account/login?returnTo=/account/privacy", await getLocale()));
  return new Response(json, {
    headers: {
      "Content-Type": "application/json",
      "Content-Disposition": 'attachment; filename="my-data.json"',
      // Full PII bundle - no shared cache, no disk cache, and never keyed without the
      // session that produced it
      "Cache-Control": "no-store, private, max-age=0, must-revalidate",
      Pragma: "no-cache",
      Vary: "Cookie, Authorization",
    },
  });
}
