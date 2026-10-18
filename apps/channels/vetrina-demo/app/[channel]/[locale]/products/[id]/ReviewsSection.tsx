"use client";

// PDP reviews block - the parent fetches the public review data so ratings and bodies land
// in the cached HTML. Per-customer bits (eligibility, own votes, signed-in state) can't be
// server-read on a cached page, so they upgrade in place after hydration
// Sort and media filter are CLIENT STATE, never URL params - the buttons emit no hrefs, so
// crawlers see exactly one URL per product; other orders come from a post-hydration action

import { useState, useTransition } from "react";
import { useTranslations } from "next-intl";

import { Link } from "@/app/components/LocaleLink";

import { useCustomerIdentity } from "@/app/lib/customer/client";
import { voteReviewAction } from "@/app/lib/reviews/actions";
import { loadReviewsPage } from "@/app/lib/reviews/actions-client";
import type { ReviewSort } from "@/app/lib/reviews/operations.gql";
import { useReviewState } from "./ReviewStateProvider";
import type { ProductReviews, ReviewListItem } from "@/app/lib/reviews/read";

type SearchParams = Record<string, string | string[] | undefined>;

export function ReviewsSection({
  productId,
  basePath,
  searchParams,
  reviewsData,
}: {
  productId: string;
  // The PDP's canonical pretty URL (from productHref)
  basePath: string;
  searchParams: SearchParams;
  reviewsData: ProductReviews | null;
}) {
  // Hooks before the soft-fail return - identity/eligibility resolve after hydration
  const signedIn = useCustomerIdentity().identity != null;
  const t = useTranslations("product.reviews");
  const { eligibility } = useReviewState();
  const [sort, setSort] = useState<ReviewSort>("NEWEST");
  // null = the server-rendered NEWEST page; a fetched page replaces it wholesale
  const [sorted, setSorted] = useState<ReviewListItem[] | null>(null);
  const [withMediaOnly, setWithMediaOnly] = useState(false);
  const [pending, startTransition] = useTransition();

  if (!reviewsData) {
    // Soft-fail: don't break the PDP if reviews subsystem misbehaves
    return null;
  }

  const changeSort = (next: ReviewSort) => {
    if (next === sort || pending) return;
    const prev = sort;
    setSort(next);
    startTransition(async () => {
      if (next === "NEWEST") {
        setSorted(null);
        return;
      }
      const rows = await loadReviewsPage(productId, next);
      // Soft-fail: keep what's on screen and put the highlight back where it was
      if (rows) setSorted(rows);
      else setSort(prev);
    });
  };

  const showThanks = pickString(searchParams.reviewSubmitted) === "1";
  const shown = sorted ?? reviewsData.reviews;
  const filtered = withMediaOnly
    ? shown.filter((r) => r.media.length > 0)
    : shown;

  // When the admin locks voting to signed-in users, anonymous viewers see a sign-in
  // nudge instead of the thumb buttons
  const canVote =
    signedIn ||
    !reviewsData.reviewAggregates.votingPolicy.requireAuthentication;

  return (
    <section id="reviews" className="mt-14 scroll-mt-28 border-t border-border pt-10">
      <div className="flex flex-wrap items-baseline justify-between gap-3">
        <h2 className="text-2xl font-extrabold tracking-tight">
          {t("title")}
        </h2>
        <p className="text-sm text-muted-foreground">
          {reviewsData.reviewAggregates.count}{" "}
          {reviewsData.reviewAggregates.count === 1 ? "review" : "reviews"}
        </p>
      </div>

      {showThanks ? (
        <div className="mt-4 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm">
          {t("thanks")}
        </div>
      ) : null}

      <SummaryStrip
        count={reviewsData.reviewAggregates.count}
        overall={reviewsData.reviewAggregates.overallAverage}
        distribution={reviewsData.reviewAggregates.distribution}
        perQuestion={reviewsData.reviewAggregates.perQuestion}
      />

      {reviewsData.reviewSummary ? (
        <div className="mt-6 rounded-2xl border border-border bg-card p-5">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {t("whatCustomersSay")}
          </p>
          <p className="mt-2 text-sm leading-relaxed text-foreground">
            {reviewsData.reviewSummary}
          </p>
          <p className="mt-2 text-[10px] text-muted-foreground">{t("aiSummary")}</p>
        </div>
      ) : null}

      <FilterBar
        productId={productId}
        sort={sort}
        onSortChange={changeSort}
        pending={pending}
        withMediaOnly={withMediaOnly}
        onMediaToggle={() => setWithMediaOnly((v) => !v)}
        eligibility={eligibility ?? reviewsData.customerCanReview}
      />

      {filtered.length === 0 ? (
        <p className="mt-8 rounded-lg border border-dashed border-border p-10 text-center text-sm text-muted-foreground">
          {withMediaOnly
            ? t("noneWithMedia")
            : t("none")}
        </p>
      ) : (
        <ul
          aria-busy={pending || undefined}
          className={`mt-8 grid gap-4 transition-opacity ${pending ? "opacity-50" : ""}`}
        >
          {filtered.map((r) => (
            <li key={r.id}>
              <ReviewCard
                review={r}
                productId={productId}
                basePath={basePath}
                canVote={canVote}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

// Full-width band above the review list - rating at a glance and the trait breakdown
// belong here, never in a side rail
function SummaryStrip({
  count,
  overall,
  distribution,
  perQuestion,
}: {
  count: number;
  overall: number | null | undefined;
  distribution: readonly number[];
  perQuestion: {
    question: { id: string; label: string };
    average: number;
    count: number;
  }[];
}) {
  const t = useTranslations("product.reviews");
  const total = count || 0;
  return (
    <div className="mt-6 grid gap-6 rounded-2xl border border-border bg-muted p-6 lg:grid-cols-[auto_1fr_1fr] lg:items-center lg:gap-10">
      <div className="flex items-center gap-5 lg:flex-col lg:items-start lg:gap-2">
        <div>
          <p className="text-5xl font-semibold leading-none tabular-nums tracking-tight">
            {overall ? overall.toFixed(1) : "-"}
            <span className="ml-1 text-lg font-normal text-muted-foreground">
              / 5
            </span>
          </p>
          <p className="mt-2 text-xs uppercase tracking-wider text-muted-foreground">
            {t("count", { count: total })}
          </p>
        </div>
        <Stars value={overall ?? 0} size={20} />
      </div>

      <div className="grid gap-1.5">
        {[5, 4, 3, 2, 1].map((star) => {
          const c = distribution[star - 1] ?? 0;
          const pct = total > 0 ? (c / total) * 100 : 0;
          return (
            <div key={star} className="flex items-center gap-3 text-xs">
              <span className="w-6 text-muted-foreground tabular-nums">{star}★</span>
              <div className="h-2 flex-1 overflow-hidden rounded-full bg-border">
                <div
                  className="h-full rounded-full bg-amber-500"
                  style={{ width: `${pct.toFixed(1)}%` }}
                />
              </div>
              <span className="w-8 text-right text-muted-foreground tabular-nums">
                {c}
              </span>
            </div>
          );
        })}
      </div>

      {perQuestion.length > 0 ? (
        <div className="grid gap-3">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {t("byQuestion")}
          </p>
          <ul className="grid gap-2.5 text-sm">
            {perQuestion.map((p) => {
              const pct = (p.average / 5) * 100;
              return (
                <li key={p.question.id} className="grid gap-1">
                  <div className="flex items-baseline justify-between gap-2 text-xs">
                    <span className="truncate text-muted-foreground">
                      {p.question.label}
                    </span>
                    <span className="shrink-0 font-medium tabular-nums">
                      {p.average.toFixed(1)}
                    </span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-border">
                    <div
                      className="h-full rounded-full bg-foreground"
                      style={{ width: `${pct.toFixed(1)}%` }}
                    />
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      ) : (
        <div className="text-xs text-muted-foreground">
          {t("noPerQuestion")}
        </div>
      )}
    </div>
  );
}

function FilterBar({
  productId,
  sort,
  onSortChange,
  pending,
  withMediaOnly,
  onMediaToggle,
  eligibility,
}: {
  productId: string;
  sort: ReviewSort;
  onSortChange: (next: ReviewSort) => void;
  pending: boolean;
  withMediaOnly: boolean;
  onMediaToggle: () => void;
  eligibility: {
    canReview: boolean;
    reason?: string | null;
    existingReviewId?: string | null;
  };
}) {
  return (
    <div className="mt-6 flex flex-wrap items-center justify-between gap-4 border-y border-border py-3">
      <div className="flex flex-wrap items-center gap-3 text-xs">
        <SortControl current={sort} onChange={onSortChange} pending={pending} />
        <MediaOnlyToggle on={withMediaOnly} onToggle={onMediaToggle} />
      </div>
      <WriteReviewCTA productId={productId} eligibility={eligibility} />
    </div>
  );
}

function WriteReviewCTA({
  productId,
  eligibility,
}: {
  productId: string;
  eligibility: {
    canReview: boolean;
    reason?: string | null;
    existingReviewId?: string | null;
  };
}) {
  const t = useTranslations("product.reviews");
  if (eligibility.canReview) {
    return (
      <Link
        href={`/products/${productId}/review`}
        className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
      >
        {t("write")}
      </Link>
    );
  }
  switch (eligibility.reason) {
    case "NOT_AUTHENTICATED": {
      const returnTo = encodeURIComponent(`/products/${productId}/review`);
      return (
        <Link
          href={`/account/login?returnTo=${returnTo}`}
          className="text-sm underline underline-offset-2"
        >
          {t("signInToWrite")}
        </Link>
      );
    }
    case "NOT_PURCHASED":
      return (
        <p className="text-sm text-muted-foreground">
          {t("notPurchased")}
        </p>
      );
    case "ALREADY_REVIEWED":
      return (
        <Link
          href={`/products/${productId}/review`}
          className="text-sm underline underline-offset-2"
        >
          {t("edit")}
        </Link>
      );
    default:
      return null;
  }
}

// Buttons on purpose, never links - a crawler must not discover per-sort URL variants
function SortControl({
  current,
  onChange,
  pending,
}: {
  current: ReviewSort;
  onChange: (next: ReviewSort) => void;
  pending: boolean;
}) {
  const t = useTranslations("product.reviews");
  const options: { value: ReviewSort; label: string }[] = [
    { value: "NEWEST", label: t("newest") },
    { value: "OLDEST", label: t("oldest") },
    { value: "HIGHEST", label: t("highest") },
    { value: "LOWEST", label: t("lowest") },
  ];
  return (
    <span className="flex items-center gap-1">
      <span className="text-muted-foreground">{t("sort")}</span>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          disabled={pending}
          aria-pressed={current === o.value}
          className={`rounded-md px-3 py-1 transition-colors ${
            current === o.value
              ? "bg-foreground text-background"
              : "text-muted-foreground hover:bg-muted hover:text-foreground"
          }`}
        >
          {o.label}
        </button>
      ))}
    </span>
  );
}

function MediaOnlyToggle({
  on,
  onToggle,
}: {
  on: boolean;
  onToggle: () => void;
}) {
  const t = useTranslations("product.reviews");
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={on}
      className={`rounded-md px-3 py-1 transition-colors ${
        on
          ? "bg-foreground text-background"
          : "text-muted-foreground hover:bg-muted hover:text-foreground"
      }`}
    >
      {t("withMedia")}
    </button>
  );
}

function ReviewCard({
  review,
  productId,
  basePath,
  canVote,
}: {
  review: ReviewListItem;
  productId: string;
  basePath: string;
  canVote: boolean;
}) {
  const t = useTranslations("product.reviews");
  const tc = useTranslations("common");
  const overall = review.overallRating ?? null;
  return (
    <article className="rounded-xl border border-border bg-card p-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <Avatar name={review.customerDisplayName} />
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium">
                {review.customerDisplayName || t("customer")}
              </span>
              {review.isVerifiedPurchase ? (
                <span className="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">
                  {t("verified")}
                </span>
              ) : null}
            </div>
            <time
              dateTime={review.submittedAt}
              className="text-xs text-muted-foreground"
            >
              {new Date(review.submittedAt).toLocaleDateString(undefined, {
                year: "numeric",
                month: "long",
                day: "numeric",
              })}
            </time>
          </div>
        </div>
        {overall != null ? (
          <div className="flex items-center gap-2">
            <Stars value={overall} size={16} />
            <span className="text-sm font-medium tabular-nums">
              {overall.toFixed(1)}
            </span>
          </div>
        ) : null}
      </header>

      {review.title ? (
        <p className="mt-3 text-base font-medium">{review.title}</p>
      ) : null}
      {review.body ? (
        <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
          {review.body}
        </p>
      ) : null}

      {review.answers.length > 0 ? (
        <ul className="mt-4 grid gap-1.5 rounded-lg bg-muted p-3 text-xs">
          {review.answers
            .filter((a) => a.rating != null || a.boolValue != null || a.text)
            .map((a, i) => (
              <li key={i} className="flex items-baseline gap-2">
                <span className="font-medium text-muted-foreground">
                  {a.question.label}:
                </span>
                {a.rating != null ? (
                  <span className="tabular-nums">{a.rating} / 5</span>
                ) : a.boolValue != null ? (
                  <span>{a.boolValue ? tc("yes") : tc("no")}</span>
                ) : null}
                {a.text ? (
                  <span className="text-muted-foreground">
                    - {a.text}
                  </span>
                ) : null}
              </li>
            ))}
        </ul>
      ) : null}

      {review.media.length > 0 ? (
        <ul className="mt-4 flex flex-wrap gap-2">
          {review.media.map((m) => (
            <li key={m.id}>
              {m.mimeType.startsWith("video/") ? (
                <video
                  controls
                  src={m.url}
                  className="h-24 w-24 rounded-lg border bg-black object-cover"
                />
              ) : (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={m.url}
                  alt=""
                  className="h-24 w-24 rounded-lg border object-cover"
                />
              )}
            </li>
          ))}
        </ul>
      ) : null}

      {review.merchantReply ? (
        <div className="mt-4 rounded-lg border-l-2 border-border bg-muted p-3">
          <p className="text-xs font-semibold text-muted-foreground">{t("sellerResponse")}</p>
          <p className="mt-1 whitespace-pre-wrap text-sm text-foreground">
            {review.merchantReply}
          </p>
        </div>
      ) : null}

      <VoteRow
        reviewId={review.id}
        productId={productId}
        basePath={basePath}
        helpfulCount={review.helpfulCount}
        notHelpfulCount={review.notHelpfulCount}
        myVote={review.myVote}
        canVote={canVote}
      />
    </article>
  );
}

// Each thumb is its own form so a no-JS browser still votes. value carries the desired state: +1/-1, or 0 to toggle an
// active vote back off canVote false means anon visitors see counts and a sign-in link instead
function VoteRow({
  reviewId,
  productId,
  basePath,
  helpfulCount,
  notHelpfulCount,
  myVote,
  canVote,
}: {
  reviewId: string;
  productId: string;
  basePath: string;
  helpfulCount: number;
  notHelpfulCount: number;
  myVote: number;
  canVote: boolean;
}) {
  // Server-rendered value is the anonymous one baked into the cached page - the resolved
  // map replaces it once the visitor's own votes come back
  const resolved = useReviewState().votes[reviewId] ?? myVote;
  const t = useTranslations("product.reviews");
  const helpfulActive = resolved === 1;
  const notHelpfulActive = resolved === -1;
  return (
    <footer className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3 text-xs">
      <span className="text-muted-foreground">{t("helpfulQuestion")}</span>
      {canVote ? (
        <div className="flex items-center gap-2">
          <form action={voteReviewAction}>
            <input type="hidden" name="reviewId" value={reviewId} />
            <input type="hidden" name="productId" value={productId} />
            <input
              type="hidden"
              name="value"
              value={helpfulActive ? 0 : 1}
            />
            <VoteButton
              direction="up"
              count={helpfulCount}
              active={helpfulActive}
              label={t("helpful")}
            />
          </form>
          <form action={voteReviewAction}>
            <input type="hidden" name="reviewId" value={reviewId} />
            <input type="hidden" name="productId" value={productId} />
            <input
              type="hidden"
              name="value"
              value={notHelpfulActive ? 0 : -1}
            />
            <VoteButton
              direction="down"
              count={notHelpfulCount}
              active={notHelpfulActive}
              label={t("notHelpful")}
            />
          </form>
        </div>
      ) : (
        <div className="flex items-center gap-3 text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <ThumbIcon direction="up" /> {helpfulCount}
          </span>
          <span className="inline-flex items-center gap-1">
            <ThumbIcon direction="down" /> {notHelpfulCount}
          </span>
          <Link
            href={`/account/login?returnTo=${encodeURIComponent(basePath)}`}
            className="underline underline-offset-2"
          >
            {t("signInToVote")}
          </Link>
        </div>
      )}
    </footer>
  );
}

function VoteButton({
  direction,
  count,
  active,
  label,
}: {
  direction: "up" | "down";
  count: number;
  active: boolean;
  label: string;
}) {
  return (
    <button
      type="submit"
      aria-label={label}
      aria-pressed={active}
      className={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1 transition-colors ${
        active
          ? direction === "up"
            ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
            : "border-rose-500/40 bg-rose-500/10 text-rose-700 dark:text-rose-300"
          : "border-border text-muted-foreground hover:bg-accent"
      }`}
    >
      <ThumbIcon direction={direction} />
      <span className="tabular-nums">{count}</span>
    </button>
  );
}

function ThumbIcon({ direction }: { direction: "up" | "down" }) {
  // Inline SVG keeps this server-component-safe (no heroicon import) and small enough
  // that the bundle stays untouched
  const flip = direction === "down" ? "rotate(180)" : undefined;
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      transform={flip}
      aria-hidden
    >
      <path d="M7 22V11" />
      <path d="M14 4.5a3 3 0 0 0-6 0V11h12.5a2.5 2.5 0 0 1 2.45 3l-1.42 7A2.5 2.5 0 0 1 19.08 23H7" />
    </svg>
  );
}

function Stars({ value, size }: { value: number; size: number }) {
  const clamped = Math.max(0, Math.min(5, value));
  const pct = (clamped / 5) * 100;
  const fontStyle = { fontSize: `${size}px`, lineHeight: 1 } as const;
  return (
    <span
      className="relative inline-block select-none leading-none"
      style={fontStyle}
      aria-label={`${clamped.toFixed(1)} out of 5 stars`}
    >
      <span className="text-border">★★★★★</span>
      <span
        className="absolute inset-0 overflow-hidden whitespace-nowrap text-amber-500"
        style={{ width: `${pct}%` }}
        aria-hidden
      >
        ★★★★★
      </span>
    </span>
  );
}

function Avatar({ name }: { name: string }) {
  const initials = (name || "C")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((s) => s[0]!.toUpperCase())
    .join("");
  // Deterministic hue from the name so the same reviewer always gets the same colour -
  // small consistency win on multi-review browses
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) | 0;
  const hue = Math.abs(hash) % 360;
  return (
    <div
      className="grid h-10 w-10 shrink-0 place-items-center rounded-full text-xs font-semibold text-white"
      style={{ backgroundColor: `hsl(${hue} 55% 45%)` }}
      aria-hidden
    >
      {initials || "C"}
    </div>
  );
}

function pickString(v: string | string[] | undefined): string {
  if (Array.isArray(v)) return v[0] ?? "";
  return v ?? "";
}

