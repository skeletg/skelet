"use server";

import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { loadHeader } from "@/app/lib/header/read";
import { getCurrentChannelLocales } from "@/app/lib/locale/read";

// Switching language changes the URL, it does not set a cookie. The pages that matter here render
// without a request and can't read one, so a cookie-driven language silently served everyone the
// default - the prefix is part of the cache key instead, and each language gets its own page
export async function setLocale(formData: FormData) {
  const code = String(formData.get("code") ?? "").trim();
  if (!code) return;
  // Validate against the active channel's locale list - the same data the header picker renders from (loadHeader is per-request cached, and
  // its `locales(activeOnly: true)` query is resolved against the current store via the X-Channel-Id header)
  try {
    const { locales } = await loadHeader();
    if (!locales.some((l) => l.code === code)) return;
  } catch {
    return;
  }

  // Stay on the page they were reading rather than dumping them on the homepage. A server action
  // has no url of its own, so the referer is the only handle on where the form was submitted from
  const referer = (await headers()).get("referer") ?? "";
  let path = "/";
  try {
    path = new URL(referer).pathname || "/";
  } catch {
    // no or malformed referer - fall back to the homepage in the requested language
  }

  // Drop an existing prefix before adding the new one, or switching twice stacks them
  const { codes, defaultLocale } = await getCurrentChannelLocales();
  const first = path.split("/")[1] ?? "";
  if (codes.includes(first)) path = path.slice(first.length + 1) || "/";
  redirect(code === defaultLocale ? path : `/${code}${path === "/" ? "" : path}`);
}
