import type { Block } from "./blocks";
import { readString } from "./blocks";

export function HtmlBlock({ block }: { block: Block }) {
  const html = readString(block.props, "html");
  if (!html) return null;
  return (
    <section className="border-b border-border">
      <div
        className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14 text-base leading-relaxed text-foreground"
        // Sanitized server-side by blockkit before storage - see component note
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </section>
  );
}
