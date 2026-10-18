import { afterEach, describe, expect, it, vi } from "vitest";

import { signVoterToken, voterTokenSecret } from "@/app/lib/reviews/voter-token";

afterEach(() => vi.unstubAllEnvs());

describe("voter tokens", () => {
  // Core's review.ParseVoterToken verifies these, so the bytes have to agree with the Go side exactly
  it("signs the uuid's raw bytes the way core verifies them", () => {
    vi.stubEnv("REVIEW_VOTER_TOKEN_SECRET", "review-voter-secret-0123456789ab");
    expect(signVoterToken("7f1c2b9e-3d4a-4c5b-8e6f-0a1b2c3d4e5f")).toBe(
      "7f1c2b9e-3d4a-4c5b-8e6f-0a1b2c3d4e5f.T_cUvNzohQcHKVs78HBkxHrxriRElUTXhFJpF381kRc",
    );
  });

  // The fallback put the admin signing key in the storefront container, so a JWT_SECRET alone must not be enough
  it("needs its own secret and ignores JWT_SECRET", () => {
    vi.stubEnv("REVIEW_VOTER_TOKEN_SECRET", "");
    vi.stubEnv("JWT_SECRET", "a-shared-session-secret-0123456789");
    expect(() => voterTokenSecret()).toThrow(/REVIEW_VOTER_TOKEN_SECRET/);
  });
});
