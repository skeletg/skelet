"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { MessageCircleQuestionIcon } from "lucide-react";
import { useTranslations } from "next-intl";

// The panel carries the form machinery (zod, Conform) - loaded when the shopper asks, or on hover of
// the button so it is usually there by the click
const loadPanel = () => import("./AskQuestionPanel").then((m) => m.AskQuestionPanel);
const AskQuestionPanel = dynamic(loadPanel, {
  loading: () => <div className="glass h-48 max-w-xl rounded-lg" aria-busy="true" />,
});

export function AskQuestionForm({ productId }: { productId: string }) {
  const t = useTranslations("product.qna");
  const [open, setOpen] = useState(false);

  if (open) {
    return <AskQuestionPanel productId={productId} onCancel={() => setOpen(false)} />;
  }

  return (
    <button
      type="button"
      onClick={() => setOpen(true)}
      onPointerEnter={() => void loadPanel()}
      onFocus={() => void loadPanel()}
      className="inline-flex h-10 items-center gap-2 rounded-md border border-foreground/25 px-5 text-sm font-semibold transition hover:border-foreground"
    >
      <MessageCircleQuestionIcon className="h-4 w-4" aria-hidden="true" />
      {t("ask")}
    </button>
  );
}
