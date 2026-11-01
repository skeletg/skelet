import type { Block } from "./blocks";
import { readString } from "./blocks";
import { LexicalRichText } from "@/app/lib/lexical-render";
import { lexicalHasContent } from "@skelet/storefront-core/lexical";

const TONES: Record<string, { frame: string; title: string }> = {
  info: { frame: "border-sky-400/50 bg-sky-50 dark:bg-sky-950/30", title: "text-sky-900 dark:text-sky-200" },
  success: { frame: "border-emerald-400/50 bg-emerald-50 dark:bg-emerald-950/30", title: "text-emerald-900 dark:text-emerald-200" },
  warning: { frame: "border-amber-400/60 bg-amber-50 dark:bg-amber-950/30", title: "text-amber-900 dark:text-amber-200" },
  danger: { frame: "border-red-400/50 bg-red-50 dark:bg-red-950/30", title: "text-red-900 dark:text-red-200" },
};

export function CalloutBlock({ block }: { block: Block }) {
  const tone = readString(block.props, "tone") ?? "info";
  const title = readString(block.props, "title");
  const body = block.props.body;
  const hasBody = lexicalHasContent(body);
  if (!title && !hasBody) return null;
  const t = TONES[tone] ?? TONES.info;

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-3xl px-6 py-8">
        <div className={`rounded-xl border p-5 ${t.frame}`}>
          {title && <p className={`text-sm font-semibold ${t.title}`}>{title}</p>}
          {hasBody && (
            <div className={`text-sm leading-relaxed text-foreground/90 ${title ? "mt-2" : ""}`}>
              <LexicalRichText value={body} />
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
