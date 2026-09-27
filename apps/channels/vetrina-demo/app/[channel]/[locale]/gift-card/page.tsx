// Public gift card balance checker

import { GiftCardBalanceDocument } from "@/app/lib/checkout/operations.gql";
import { formatPrice } from "@/app/lib/format";
import { gqlFetch } from "@/app/lib/graphql";

export const metadata = { title: "Gift card balance" };

export const dynamic = "force-dynamic";

type RawSearchParams = Record<string, string | string[] | undefined>;

function pickCode(sp: RawSearchParams): string {
  const raw = sp.code;
  if (typeof raw === "string") return raw.trim();
  if (Array.isArray(raw)) return (raw[0] ?? "").trim();
  return "";
}

type BalanceResult =
  | { kind: "found"; balanceCents: number; currencyCode: string; active: boolean }
  | { kind: "not_found" }
  | { kind: "error" };

export default async function GiftCardPage({
  searchParams,
}: {
  searchParams: Promise<RawSearchParams>;
}) {
  const sp = await searchParams;
  const code = pickCode(sp);

  let result: BalanceResult | null = null;
  if (code) {
    try {
      const res = await gqlFetch(GiftCardBalanceDocument, { code });
      result = res.data.giftCardBalance
        ? { kind: "found", ...res.data.giftCardBalance }
        : { kind: "not_found" };
    } catch {
      // Backend refused (rate limit) or is unreachable - same soft copy
      result = { kind: "error" };
    }
  }

  return (
    <main className="mx-auto w-full max-w-8xl px-6 py-16 md:px-10">
      <div className="glass mx-auto w-full max-w-md rounded-xl p-8">
        <h1 className="text-2xl font-extrabold tracking-tight">
          Gift card balance
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Enter the full code from your gift card to see what&rsquo;s
          left on it.
        </p>

        <form action="/gift-card" method="GET" className="mt-6 flex gap-2">
          <input
            type="text"
            name="code"
            defaultValue={code}
            required
            placeholder="Gift card code"
            autoComplete="off"
            spellCheck={false}
            className="min-w-0 flex-1 rounded-md border border-border bg-background px-4 py-2 text-sm outline-none focus:border-foreground"
          />
          <button
            type="submit"
            className="inline-flex h-10 shrink-0 items-center justify-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground hover:bg-primary/90"
          >
            Check balance
          </button>
        </form>

        {result?.kind === "found" && (
          <div className="mt-6 rounded-lg bg-muted/60 p-5">
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Remaining balance
            </p>
            <p className="mt-1 flex items-center gap-3">
              <span className="text-3xl font-semibold tabular-nums tracking-tight">
                {formatPrice(result.balanceCents, result.currencyCode)}
              </span>
              {result.active ? (
                <span className="rounded-sm bg-emerald-100 px-2.5 py-0.5 text-xs font-medium text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300">
                  Active
                </span>
              ) : (
                <span className="rounded-sm bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground">
                  Inactive
                </span>
              )}
            </p>
            {result.active ? (
              <p className="mt-2 text-xs text-muted-foreground">
                Apply the code at checkout to spend it.
              </p>
            ) : (
              <p className="mt-2 text-xs text-muted-foreground">
                This card can&rsquo;t be used right now - contact support
                if that seems wrong.
              </p>
            )}
          </div>
        )}

        {result?.kind === "not_found" && (
          <p className="mt-6 rounded-lg bg-muted/60 p-4 text-sm text-muted-foreground">
            No gift card matches that code. Double-check for typos - the
            full code is on your gift card email or receipt.
          </p>
        )}

        {result?.kind === "error" && (
          <p className="mt-6 rounded-lg bg-muted/60 p-4 text-sm text-muted-foreground">
            We couldn&rsquo;t check that code right now. Wait a minute
            and try again.
          </p>
        )}
      </div>
    </main>
  );
}
