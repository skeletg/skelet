import type { Block } from "./blocks";
import { LexicalRichText, lexicalHasContent } from "@/app/lib/lexical-render";

export function RichTextBlock({ block }: { block: Block }) {
  const content = block.props.content;
  if (!lexicalHasContent(content)) return null;
  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-3xl px-6 py-12 text-base leading-relaxed text-foreground">
        <LexicalRichText value={content} />
      </div>
    </section>
  );
}
