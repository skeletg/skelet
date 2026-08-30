import { getProductQuestions } from "@/app/lib/qna/actions";
import { formatDate } from "@/app/lib/format";
import { AskQuestionForm } from "./AskQuestionForm";

// Full-width split: heading + ask-form live in a left rail, the answered list owns the
// rest. The Q/A letters are plates, not text prefixes - ink for the shopper's question,
// signal orange for the store's answer, so the pair reads at a glance
export async function QnaSection({ productId }: { productId: string }) {
  const questions = await getProductQuestions(productId);

  return (
    <section
      id="qna"
      className="mt-14 scroll-mt-28 border-t border-border pt-10"
    >
      <div className="grid gap-x-12 gap-y-8 lg:grid-cols-[minmax(0,4fr)_minmax(0,8fr)]">
        <div>
          <h2 className="text-2xl font-extrabold tracking-tight">
            Questions &amp; answers
          </h2>
          <p className="mt-2 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">
            {questions.length === 0
              ? "Answered by the store team"
              : `${questions.length} answered by the store team`}
          </p>
          <p className="mt-4 max-w-sm text-sm text-muted-foreground">
            Something the description doesn&rsquo;t cover? Ask - answers are
            published here for everyone.
          </p>
          <div className="mt-5">
            <AskQuestionForm productId={productId} />
          </div>
        </div>

        {questions.length === 0 ? (
          <div className="flex min-h-32 items-center justify-center rounded-lg border border-dashed border-border p-8 text-center">
            <p className="text-sm text-muted-foreground">
              No questions yet. Ask the first one.
            </p>
          </div>
        ) : (
          <ul className="divide-y divide-border">
            {questions.map((q) => (
              <li key={q.id} className="py-6 first:pt-0 last:pb-0">
                <div className="flex items-start gap-3">
                  <span
                    aria-hidden="true"
                    className="flex size-7 shrink-0 items-center justify-center rounded-sm bg-foreground font-mono text-xs font-bold text-background"
                  >
                    Q
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold leading-snug">
                      <span className="sr-only">Question: </span>
                      {q.question}
                    </p>
                    <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.06em] text-muted-foreground">
                      {q.authorName || "Anonymous"} · {formatDate(q.createdAt)}
                    </p>
                  </div>
                </div>
                <div className="mt-4 flex items-start gap-3">
                  <span
                    aria-hidden="true"
                    className="flex size-7 shrink-0 items-center justify-center rounded-sm bg-signal font-mono text-xs font-bold text-white"
                  >
                    A
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="whitespace-pre-line text-sm leading-relaxed">
                      <span className="sr-only">Answer: </span>
                      {q.answer}
                    </p>
                    <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.06em] text-muted-foreground">
                      Store team
                      {q.answeredAt && <> · {formatDate(q.answeredAt)}</>}
                    </p>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
