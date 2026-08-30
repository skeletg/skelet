"use client";

import { useActionState, useRef, useState, type ChangeEvent } from "react";

import type { ReviewQuestionWire } from "@/app/lib/reviews/read";
import {
  requestReviewMediaUploadAction,
  submitReviewAction,
} from "@/app/lib/reviews/actions";

type MediaItem =
  | { kind: "uploading"; key: string; filename: string }
  | { kind: "ready"; assetId: string; filename: string; previewUrl: string; mimeType: string }
  | { kind: "error"; key: string; filename: string; error: string };

export function ReviewForm({
  productId,
  questions,
  isGuest,
}: {
  productId: string;
  questions: ReviewQuestionWire[];
  // When true, the visitor has no JWT cookie
  isGuest: boolean;
}) {
  const [state, formAction, isPending] = useActionState(submitReviewAction, null);
  const [media, setMedia] = useState<MediaItem[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [ratings, setRatings] = useState<Record<string, number>>({});
  const [bools, setBools] = useState<Record<string, boolean>>({});
  const [texts, setTexts] = useState<Record<string, string>>({});

  async function onFilesChange(e: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? []);
    if (files.length === 0) return;
    if (fileInputRef.current) fileInputRef.current.value = "";

    for (const file of files) {
      const key = `${file.name}-${file.size}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
      setMedia((prev) => [...prev, { kind: "uploading", key, filename: file.name }]);
      try {
        const req = await requestReviewMediaUploadAction({
          productId,
          filename: file.name,
          mimeType: file.type || "application/octet-stream",
          sizeBytes: file.size,
        });
        if (!req.ok) throw new Error(req.error);
        const put = await fetch(req.uploadUrl, {
          method: "PUT",
          body: file,
          headers: { "Content-Type": file.type || "application/octet-stream" },
        });
        if (!put.ok) throw new Error(`Upload failed (HTTP ${put.status})`);
        const previewUrl = URL.createObjectURL(file);
        setMedia((prev) =>
          prev.map((m) =>
            m.kind === "uploading" && m.key === key
              ? {
                  kind: "ready",
                  assetId: req.assetId,
                  filename: file.name,
                  previewUrl,
                  mimeType: file.type || "application/octet-stream",
                }
              : m,
          ),
        );
      } catch (err) {
        const message = err instanceof Error ? err.message : "Upload failed";
        setMedia((prev) =>
          prev.map((m) =>
            m.kind === "uploading" && m.key === key
              ? { kind: "error", key, filename: file.name, error: message }
              : m,
          ),
        );
      }
    }
  }

  function removeMedia(key: string) {
    setMedia((prev) => prev.filter((m) => (m.kind === "ready" ? m.assetId !== key : m.key !== key)));
  }

  const readyAssetIds = media
    .filter((m): m is Extract<MediaItem, { kind: "ready" }> => m.kind === "ready")
    .map((m) => m.assetId);

  // Required questions block submission
  const missingRequired = questions
    .filter((q) => q.isRequired)
    .filter((q) => {
      switch (q.inputType) {
        case "RATING_1_5":
          return !ratings[q.id];
        case "YES_NO":
          return bools[q.id] === undefined;
        case "FREE_TEXT":
          return !(texts[q.id] ?? "").trim();
        default:
          return false;
      }
    });

  return (
    <form action={formAction} className="mt-6 grid gap-6">
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="mediaAssetIds" value={readyAssetIds.join(",")} />

      {isGuest ? (
        <fieldset className="grid gap-4 rounded-lg border p-4">
          <legend className="px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            About you
          </legend>
          <div className="grid gap-2">
            <label htmlFor="rv-name" className="text-sm font-medium">
              Display name <span className="text-red-500">*</span>
            </label>
            <input
              id="rv-name"
              name="guestName"
              type="text"
              required
              maxLength={80}
              className="rounded-md border border-border bg-transparent px-3 py-2 text-sm outline-none focus:border-ring"
              placeholder="How you want to appear on the review"
              autoComplete="name"
            />
          </div>
          <div className="grid gap-2">
            <label htmlFor="rv-email" className="text-sm font-medium">
              Email (optional)
            </label>
            <input
              id="rv-email"
              name="guestEmail"
              type="email"
              maxLength={120}
              className="rounded-md border border-border bg-transparent px-3 py-2 text-sm outline-none focus:border-ring"
              placeholder="We won't show it publicly"
              autoComplete="email"
            />
          </div>
        </fieldset>
      ) : null}

      <fieldset className="grid gap-2">
        <label htmlFor="rv-title" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Title (optional)
        </label>
        <input
          id="rv-title"
          name="title"
          type="text"
          maxLength={140}
          className="rounded-md border border-border bg-transparent px-3 py-2 text-sm outline-none focus:border-ring"
          placeholder="Summarise your experience"
        />
      </fieldset>

      <fieldset className="grid gap-2">
        <label htmlFor="rv-body" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Your review
        </label>
        <textarea
          id="rv-body"
          name="body"
          rows={4}
          className="rounded-md border border-border bg-transparent px-3 py-2 text-sm outline-none focus:border-ring"
          placeholder="What did you like or dislike?"
        />
      </fieldset>

      {questions.length > 0 ? (
        <fieldset className="grid gap-4 rounded-lg border p-4">
          <legend className="px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Rate this product
          </legend>
          {questions.map((q) => (
            <QuestionField
              key={q.id}
              question={q}
              rating={ratings[q.id]}
              setRating={(v) => setRatings((p) => ({ ...p, [q.id]: v }))}
              bool={bools[q.id]}
              setBool={(v) => setBools((p) => ({ ...p, [q.id]: v }))}
              text={texts[q.id] ?? ""}
              setText={(v) => setTexts((p) => ({ ...p, [q.id]: v }))}
            />
          ))}
        </fieldset>
      ) : null}

      <fieldset className="grid gap-2">
        <label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Photos &amp; videos (optional)
        </label>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*,video/mp4,video/webm,video/quicktime"
          multiple
          onChange={onFilesChange}
          className="text-sm"
        />
        {media.length > 0 ? (
          <ul className="mt-2 grid grid-cols-3 gap-2 sm:grid-cols-4">
            {media.map((m) => (
              <li key={m.kind === "ready" ? m.assetId : m.key} className="relative rounded-md border p-1">
                {m.kind === "ready" ? (
                  m.mimeType.startsWith("video/") ? (
                    <video src={m.previewUrl} className="aspect-square w-full rounded bg-black object-cover" />
                  ) : (
                    /* eslint-disable-next-line @next/next/no-img-element */
                    <img src={m.previewUrl} alt={m.filename} className="aspect-square w-full rounded object-cover" />
                  )
                ) : m.kind === "uploading" ? (
                  <div className="flex aspect-square w-full items-center justify-center rounded bg-muted text-[10px] text-muted-foreground">
                    Uploading…
                  </div>
                ) : (
                  <div className="flex aspect-square w-full items-center justify-center rounded bg-red-500/10 p-1 text-center text-[10px] text-red-600">
                    {m.error}
                  </div>
                )}
                <button
                  type="button"
                  onClick={() =>
                    removeMedia(m.kind === "ready" ? m.assetId : m.key)
                  }
                  className="absolute right-1 top-1 rounded-sm bg-black/60 px-1.5 text-[10px] text-white"
                  aria-label={`Remove ${m.filename}`}
                >
                  ✕
                </button>
              </li>
            ))}
          </ul>
        ) : null}
      </fieldset>

      {state && !state.ok ? (
        <p className="rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-600">
          {state.error}
        </p>
      ) : null}

      <div className="flex items-center justify-end gap-3">
        {missingRequired.length > 0 ? (
          <p className="text-xs text-amber-600">
            Required:{" "}
            {missingRequired.map((q) => q.label).join(", ")}
          </p>
        ) : null}
        <button
          type="submit"
          disabled={isPending}
          className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground"
        >
          {isPending ? "Submitting…" : "Submit review"}
        </button>
      </div>
    </form>
  );
}

function QuestionField({
  question,
  rating,
  setRating,
  bool,
  setBool,
  text,
  setText,
}: {
  question: ReviewQuestionWire;
  rating: number | undefined;
  setRating: (v: number) => void;
  bool: boolean | undefined;
  setBool: (v: boolean) => void;
  text: string;
  setText: (v: string) => void;
}) {
  return (
    <div className="grid gap-2">
      <input
        type="hidden"
        name={`answer.${question.id}.inputType`}
        value={question.inputType}
      />
      <div>
        <label className="text-sm font-medium">
          {question.label}
          {question.isRequired ? <span className="text-red-500">*</span> : null}
        </label>
        {question.helpText ? (
          <p className="text-xs text-muted-foreground">{question.helpText}</p>
        ) : null}
      </div>

      {question.inputType === "RATING_1_5" ? (
        <div className="flex items-center gap-1">
          {[1, 2, 3, 4, 5].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => setRating(n)}
              className={`flex h-9 w-9 items-center justify-center rounded text-lg transition ${
                rating != null && rating >= n
                  ? "text-amber-500"
                  : "text-border hover:text-muted-foreground"
              }`}
              aria-label={`${n} star${n === 1 ? "" : "s"}`}
            >
              ★
            </button>
          ))}
          <input
            type="hidden"
            name={`answer.${question.id}.rating`}
            value={rating ?? ""}
          />
        </div>
      ) : null}

      {question.inputType === "YES_NO" ? (
        <div className="flex items-center gap-2">
          {[
            { label: "Yes", value: true },
            { label: "No", value: false },
          ].map((opt) => (
            <button
              key={opt.label}
              type="button"
              onClick={() => setBool(opt.value)}
              className={`rounded-md border px-3 py-1 text-xs transition ${
                bool === opt.value
                  ? "border-foreground bg-foreground text-background"
                  : "border-border hover:border-ring"
              }`}
            >
              {opt.label}
            </button>
          ))}
          <input
            type="hidden"
            name={`answer.${question.id}.bool`}
            value={bool === undefined ? "" : bool ? "true" : "false"}
          />
        </div>
      ) : null}

      {question.allowComment || question.inputType === "FREE_TEXT" ? (
        <textarea
          name={`answer.${question.id}.text`}
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={2}
          placeholder={
            question.inputType === "FREE_TEXT"
              ? "Your answer"
              : "Optional comment"
          }
          className="rounded-md border border-border bg-transparent px-3 py-2 text-sm outline-none focus:border-ring"
        />
      ) : null}
    </div>
  );
}
