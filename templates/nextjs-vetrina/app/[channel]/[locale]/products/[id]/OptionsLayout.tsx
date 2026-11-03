"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";

import type { ProductCustomOption } from "@skelet/storefront-next/catalog/read";
import type { FileEntry } from "./option-files";
import { isSelect } from "./option-rules";
import { jumpToOption, OPTION_JUMP_EVENT } from "./focus-intent";

type Answers = {
  sel: Record<string, Set<string>>;
  text: Record<string, string>;
  files: Record<string, FileEntry | undefined>;
};

// What a customer gave for an option, in words - null when nothing yet
export function answerText(opt: ProductCustomOption, a: Answers): string | null {
  if (isSelect(opt.inputType)) {
    const picked = a.sel[opt.id];
    const titles = opt.values.filter((v) => picked?.has(v.id)).map((v) => v.title);
    return titles.length > 0 ? titles.join(", ") : null;
  }
  if (opt.inputType === "file") {
    const f = a.files[opt.id];
    return f?.status === "ready" ? f.filename : null;
  }
  const t = a.text[opt.id]?.trim();
  if (!t) return null;
  return opt.inputType === "date_time" ? t.replace("T", " ") : t;
}

// Every visible option at once, or one at a time - the product's choice. The steps follow the visible options, so
// a pick that shows or hides a later option changes the count as the customer goes
export function OptionsLayout({
  layout,
  options,
  answers,
  missingRequiredIds,
  renderOption,
  className = "mt-8",
}: {
  // Outer spacing only - the layouts set their own inner gaps
  className?: string;
  layout: "LIST" | "STEPS";
  options: ProductCustomOption[];
  answers: Answers;
  missingRequiredIds: readonly string[];
  renderOption: (opt: ProductCustomOption) => ReactNode;
}) {
  if (layout !== "STEPS" || options.length < 2) {
    return <section className={`${className} space-y-6`}>{options.map((opt) => renderOption(opt))}</section>;
  }
  return (
    <StepPicker className={className} options={options} answers={answers} missingRequiredIds={missingRequiredIds} renderOption={renderOption} />
  );
}

function StepPicker({
  className,
  options,
  answers,
  missingRequiredIds,
  renderOption,
}: {
  className: string;
  options: ProductCustomOption[];
  answers: Answers;
  missingRequiredIds: readonly string[];
  renderOption: (opt: ProductCustomOption) => ReactNode;
}) {
  const t = useTranslations("product.steps");
  // The step is held by option id, so a pick that adds or removes a later step doesn't move the customer - the
  // position it was at only matters when that option itself goes
  const [step, setStep] = useState<{ id: string | null; at: number }>({ id: options[0]?.id ?? null, at: 0 });
  const [reviewing, setReviewing] = useState(false);
  const found = options.findIndex((o) => o.id === step.id);
  const index = found >= 0 ? found : Math.min(step.at, options.length - 1);
  const current = options[index];
  const total = options.length;

  // A blocked add-to-cart or a "missing" link names an option - bring it up, then let the jump land once it's drawn
  useEffect(() => {
    const onJump = (e: Event) => {
      const id = (e as CustomEvent<string>).detail;
      const at = options.findIndex((o) => o.id === id);
      if (at < 0) return;
      setReviewing(false);
      setStep({ id, at });
      requestAnimationFrame(() => requestAnimationFrame(() => jumpToOption(id)));
    };
    window.addEventListener(OPTION_JUMP_EVENT, onJump);
    return () => window.removeEventListener(OPTION_JUMP_EVENT, onJump);
  }, [options]);

  const done = useMemo(() => new Set(options.filter((o) => answerText(o, answers) !== null).map((o) => o.id)), [options, answers]);
  if (!current) return null;
  const blocked = current.isRequired && missingRequiredIds.includes(current.id);
  const answered = done.has(current.id);
  const shown = reviewing ? options : options.slice(0, index);
  const go = (i: number) => {
    setReviewing(false);
    setStep({ id: options[i].id, at: i });
  };

  return (
    <section className={`${className} grid gap-4`} aria-label={t("progress", { current: reviewing ? total : index + 1, total })}>
      <div className="grid gap-2">
        <div className="flex items-baseline justify-between gap-3 text-xs">
          <span className="font-semibold uppercase tracking-wider text-muted-foreground">
            {reviewing ? t("review") : t("progress", { current: index + 1, total })}
          </span>
          {!reviewing ? <span className="truncate font-medium">{current.title}</span> : null}
        </div>
        <ol className="flex gap-1" aria-hidden="true">
          {options.map((o, i) => (
            <li
              key={o.id}
              className={[
                "h-1 flex-1 rounded-full transition-colors",
                reviewing || i < index ? "bg-foreground" : i === index ? "bg-foreground/50" : "bg-muted",
              ].join(" ")}
            />
          ))}
        </ol>
      </div>

      {shown.length > 0 ? (
        <ul className="divide-y rounded-xl border" aria-label={t("yourPicks")}>
          {shown.map((o) => {
            const i = options.indexOf(o);
            const answer = answerText(o, answers);
            return (
              <li key={o.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                <span className="w-5 shrink-0 text-center text-xs tabular-nums text-muted-foreground">{i + 1}</span>
                <span className="min-w-0 flex-1 truncate">
                  <span className="text-muted-foreground">{o.title}: </span>
                  <span className={answer ? "font-medium" : "text-muted-foreground"}>{answer ?? t("skipped")}</span>
                </span>
                <button type="button" onClick={() => go(i)} className="shrink-0 text-xs font-medium underline-offset-2 hover:underline">
                  {t("change")}
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}

      {reviewing ? (
        <p className="text-sm text-muted-foreground">{t("done")}</p>
      ) : (
        <div className="grid gap-4">
          {renderOption(current)}
          <div className="flex items-center justify-between gap-3">
            <button
              type="button"
              onClick={() => go(index - 1)}
              disabled={index === 0}
              className="rounded-lg border px-4 py-2 text-sm font-medium transition hover:bg-muted disabled:invisible"
            >
              {t("back")}
            </button>
            <div className="flex items-center gap-3">
              {blocked ? <span className="text-xs text-muted-foreground">{t("pickToContinue")}</span> : null}
              <button
                type="button"
                disabled={blocked}
                onClick={() => (index + 1 < total ? go(index + 1) : setReviewing(true))}
                className="rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition hover:opacity-90 disabled:opacity-40"
              >
                {!answered && !current.isRequired ? t("skip") : index + 1 < total ? t("next") : t("review")}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
