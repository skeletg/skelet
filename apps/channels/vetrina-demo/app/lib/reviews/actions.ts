"use server";

// Review submission + presigned media uploads
// Media bytes go browser -> object storage directly, never proxied through Next

import { createHmac } from "node:crypto";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePublicPath } from "@/app/lib/revalidate";

import { getAccessToken } from "@/app/lib/auth/session";
import { getProduct } from "@/app/lib/catalog/read";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { replyError } from "@/app/lib/forms/reply";
import { reviewSchema } from "@/app/lib/reviews/schemas";
import { productHref } from "@/app/lib/entity-href";
import type { ReviewAnswerInput } from "@/app/graphql/schema.types";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";
import {
  RequestReviewMediaUploadDocument,
  SubmitProductReviewDocument,
  VoteReviewDocument,
} from "@/app/lib/reviews/operations.gql";

// Signed anon-voter token `<uuid>.<hmac>` - the HMAC ties it to a secret both sides hold, so copying a UUID can't mint a valid one.
// httpOnly, 1 year: same browser means same token means one vote, enforced by the unique index on (review_id, voter_token)
const REVIEW_VOTER_COOKIE = "rvid";
const REVIEW_VOTER_MAX_AGE = 60 * 60 * 24 * 365;
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function voterTokenSecret(): Buffer {
  const explicit = process.env.REVIEW_VOTER_TOKEN_SECRET;
  if (explicit && explicit.length > 0) return Buffer.from(explicit, "utf8");
  const jwt = process.env.JWT_SECRET;
  if (jwt && jwt.length > 0) return Buffer.from(`vote:${jwt}`, "utf8");
  throw new Error(
    "Voter token secret missing: set REVIEW_VOTER_TOKEN_SECRET or JWT_SECRET",
  );
}

function uuidToBytes(id: string): Buffer {
  // Parse "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" → 16 raw bytes,
  // matching how the Go side feeds id[:] into the HMAC
  return Buffer.from(id.replace(/-/g, ""), "hex");
}

function signVoterToken(id: string): string {
  const mac = createHmac("sha256", voterTokenSecret());
  mac.update(uuidToBytes(id));
  // base64url with no padding - matches Go's base64.RawURLEncoding
  const sig = mac
    .digest("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return `${id}.${sig}`;
}

function looksLikeSignedVoterToken(raw: string | undefined): boolean {
  if (!raw) return false;
  const dot = raw.indexOf(".");
  if (dot <= 0 || dot === raw.length - 1) return false;
  return UUID_RE.test(raw.slice(0, dot));
}

async function ensureVoterToken(): Promise<string> {
  const store = await cookies();
  const existing = store.get(REVIEW_VOTER_COOKIE)?.value;
  if (looksLikeSignedVoterToken(existing)) return existing!;
  const freshId = crypto.randomUUID();
  const signed = signVoterToken(freshId);
  store.set(REVIEW_VOTER_COOKIE, signed, {
    path: "/",
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    maxAge: REVIEW_VOTER_MAX_AGE,
  });
  return signed;
}


export async function requestReviewMediaUploadAction(input: {
  productId: string;
  filename: string;
  mimeType: string;
  sizeBytes: number;
}): Promise<
  | { ok: true; assetId: string; uploadUrl: string; expiresAt: string }
  | { ok: false; error: string }
> {
  try {
    const res = await gqlFetch(RequestReviewMediaUploadDocument, {
      productId: input.productId,
      filename: input.filename,
      mimeType: input.mimeType,
      sizeBytes: input.sizeBytes,
    });
    return {
      ok: true,
      assetId: res.data.requestReviewMediaUpload.assetId,
      uploadUrl: res.data.requestReviewMediaUpload.uploadUrl,
      expiresAt: res.data.requestReviewMediaUpload.expiresAt as string,
    };
  } catch (err) {
    if (err instanceof GraphQLRequestError) {
      return { ok: false, error: err.errors[0]?.message ?? err.message };
    }
    return { ok: false, error: err instanceof Error ? err.message : "Upload failed" };
  }
}

// voteReviewAction toggles the signed-in customer's helpful / not- helpful vote on a
// single review
export async function voteReviewAction(formData: FormData): Promise<void> {
  const reviewId = String(formData.get("reviewId") ?? "");
  const productId = String(formData.get("productId") ?? "");
  const value = Number(formData.get("value") ?? 0);
  if (!reviewId) return;
  // Always ensure a voter token cookie exists - even when the visitor is signed in
  const voterToken = await ensureVoterToken();
  try {
    await gqlFetch(
      VoteReviewDocument,
      {
        reviewId,
        value: Number.isFinite(value)
          ? Math.max(-1, Math.min(1, Math.trunc(value)))
          : 0,
      },
      { extraHeaders: { "X-Review-Voter-Token": voterToken } },
    );
  } catch {
    // Soft-fail: a transient vote error (e.g. policy locked + no signed-in customer)
    // shouldn't break the page
  }
  if (productId) {
    // The PDP lives only at its pretty URL now (served via the catch-all), so
    // revalidate that path rather than the removed /products/<id> route
    const { product } = await getProduct(productId);
    if (product) await revalidatePublicPath(productHref(product));
  }
}

// Expects productId, title, body, comma-separated mediaAssetIds, and per-question
// answer.<id>.{rating,bool,text,inputType} - inputType picks the canonical leaf
export async function submitReviewAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const guest = !(await getAccessToken());
  const submission = parseForm(formData, reviewSchema({ guest }));
  if (submission.status !== "success") return submission.reply();
  const { productId, title, body, guestName, guestEmail } = submission.value;
  const mediaAssetIds = (submission.value.mediaAssetIds ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  const answers: ReviewAnswerInput[] = [];
  for (const [questionId, raw] of Object.entries(submission.value.answer ?? {})) {
    const a: ReviewAnswerInput = { questionId };
    if (raw.inputType === "RATING_1_5" && raw.rating) {
      const n = Number(raw.rating);
      if (Number.isFinite(n) && n >= 1 && n <= 5) a.rating = Math.round(n);
    }
    if (raw.inputType === "YES_NO" && raw.bool) {
      a.boolValue = raw.bool === "true";
    }
    if (raw.text) a.text = raw.text;
    // Skip entirely empty answers - required questions are enforced server-side, so
    // this just avoids posting noise for skipped optional questions
    if (a.rating === undefined && a.boolValue === undefined && !a.text) {
      continue;
    }
    answers.push(a);
  }

  try {
    await gqlFetch(SubmitProductReviewDocument, {
      input: {
        productId,
        title,
        body,
        answers,
        mediaAssetIds: mediaAssetIds.length > 0 ? mediaAssetIds : null,
        guestName,
        guestEmail: guestEmail ?? null,
      },
    });
  } catch (err) {
    return replyError(submission, err, "reviewFailed");
  }
  // Redirect + revalidate the product's pretty URL - the catch-all renders it; the
  // /products/<id> route was removed
  const { product } = await getProduct(productId);
  const dest = product ? productHref(product) : "/";
  await revalidatePublicPath(dest);
  redirect(`${dest}?reviewSubmitted=1`);
}
