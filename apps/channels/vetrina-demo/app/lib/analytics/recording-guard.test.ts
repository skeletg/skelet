import { readFileSync, readdirSync } from "node:fs";
import { join, relative, sep } from "node:path";

import { describe, expect, it } from "vitest";

import { isPrivateRecordingPath } from "@/app/lib/analytics/recording-policy";

// Guards the rule that customer identity never reaches session replay or heatmap snapshots. A component that renders a
// shopper's name, email or phone must sit under a private route (recording-policy), carry lens-block / lens-mask, or
// be listed below with the reason recording can't see it
//
// shortcut: matches the identity/customer/me/viewer/profile variable names the storefront uses - a value read under
// another name slips past, so name new identity sources one of these or extend IDENTITY_FIELD

const IDENTITY_FIELD = /\b(identity|customer|me|viewer|profile)\??\.(firstName|lastName|fullName|email|phone)\b/;
const RECORDING_GUARD = /\blens-(block|mask)\b/;

// Path from app/ → the reason the identity it touches is invisible to both recorders
const ALLOWED: Record<string, string> = {
  "components/BackInStockForm.tsx": "the email only pre-fills an input, and both recorders mask every input",
};

const APP = join(__dirname, "..", "..");
const LOCALE_ROOT = join(APP, "[channel]", "[locale]");

function sourceFiles(): string[] {
  return readdirSync(APP, { recursive: true, encoding: "utf8" })
    .filter((f) => f.endsWith(".tsx") && !f.endsWith(".test.tsx") && !f.split(sep).includes("node_modules"))
    .map((f) => join(APP, f));
}

// The browser path a file under app/[channel]/[locale]/ renders at, or null for shared components
function routeOf(file: string): string | null {
  const rel = relative(LOCALE_ROOT, file);
  return rel.startsWith("..") ? null : "/" + rel.split(sep).join("/");
}

describe("recording guard", () => {
  it("keeps every rendered customer identity out of replay and heatmap snapshots", () => {
    const unguarded = sourceFiles().filter((file) => {
      const src = readFileSync(file, "utf8");
      if (!IDENTITY_FIELD.test(src) || RECORDING_GUARD.test(src)) return false;
      const route = routeOf(file);
      if (route && isPrivateRecordingPath(route)) return false;
      return !(relative(APP, file).split(sep).join("/") in ALLOWED);
    });
    expect(unguarded.map((f) => relative(APP, f))).toEqual([]);
  });

  it("allows only files that still exist and still touch identity", () => {
    for (const path of Object.keys(ALLOWED)) {
      expect(IDENTITY_FIELD.test(readFileSync(join(APP, path), "utf8")), path).toBe(true);
    }
  });
});
