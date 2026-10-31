"use client";

import { useRef, useState, useTransition } from "react";
import { useTranslations } from "next-intl";

import { setCartNoteAction } from "@skelet/storefront-next/cart/actions";

export function CartNote({ initial }: { initial: string }) {
  const t = useTranslations("cart.note");
  const tc = useTranslations("common");
  const [note, setNote] = useState(initial);
  const [, start] = useTransition();
  const [saved, setSaved] = useState(false);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  function onChange(v: string) {
    setNote(v);
    setSaved(false);
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(() => {
      start(async () => {
        await setCartNoteAction(v);
        setSaved(true);
      });
    }, 700);
  }

  return (
    <section className="glass mt-8 rounded-xl p-5">
      <label className="flex flex-col gap-2">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {t("label")}{" "}
          <span className="font-normal normal-case text-muted-foreground/70">{tc("optional")}</span>
        </span>
        <textarea
          value={note}
          onChange={(e) => onChange(e.target.value)}
          onBlur={() => {
            if (debounce.current) clearTimeout(debounce.current);
            start(async () => {
              await setCartNoteAction(note);
              setSaved(true);
            });
          }}
          rows={2}
          maxLength={2000}
          placeholder={t("placeholder")}
          className="rounded-lg border border-input bg-card px-3 py-2 text-sm outline-none focus:border-ring"
        />
      </label>
      {saved && note.trim() !== "" && (
        <p className="mt-1 text-xs text-emerald-700">
          {t("saved")}
        </p>
      )}
    </section>
  );
}
