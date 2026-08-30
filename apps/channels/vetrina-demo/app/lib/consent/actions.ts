"use server";

import { cookies } from "next/headers";
import { revalidatePath } from "next/cache";

import { SetConsentDocument } from "@/app/lib/auth/operations.gql";
import { ConsentDefinitionsDocument } from "@/app/lib/customer/operations.gql";
import { CONSENT_COOKIE } from "@/app/lib/consent/read";
import { getAccessToken } from "@/app/lib/auth/session";
import { gqlFetch } from "@/app/lib/graphql";

// Writes the banner choice into a long-lived cookie, then - when the customer is signed in - replays the choice through
// the backend's per-purpose consent table so the marketing team's segmentation sees the same answer
export async function setConsentAction(choice: "all" | "essential") {
  const store = await cookies();
  store.set(CONSENT_COOKIE, choice, {
    path: "/",
    httpOnly: false, // banner reads it via cookies() (server) but a
    // future client component can also use document.cookie for an instant in-page
    // toggle
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    maxAge: 60 * 60 * 24 * 180, // 180 days; standard GDPR re-prompt window
  });

  // Replay through the backend mutation when a session is active
  if (await getAccessToken()) {
    try {
      const { data } = await gqlFetch(ConsentDefinitionsDocument);
      for (const def of data.consentDefinitions) {
        if (def.purpose !== "MARKETING") continue;
        try {
          await gqlFetch(SetConsentDocument, {
            kind: def.slug,
            optedIn: choice === "all",
          });
        } catch {
          // One failed slug doesn't sink the whole flow
        }
      }
    } catch {
      // Couldn't fetch definitions - banner cookie still landed
    }
  }

  // Refresh the layout so the banner re-renders without itself (it reads
  // getConsentChoice() at render time and hides when set)
  revalidatePath("/", "layout");
}

export async function acceptAllConsentAction() {
  await setConsentAction("all");
}

export async function essentialOnlyConsentAction() {
  await setConsentAction("essential");
}
