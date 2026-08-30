import type { Block } from "./blocks";
import { readImage, readString } from "./blocks";
import { CmsImage } from "./CmsImage";
import { getContainer } from "./variants";
import { GoalLink } from "./GoalLink";
import { LexicalRichText, lexicalHasContent } from "@/app/lib/lexical-render";

// media_text - an image beside rich text with an optional CTA. props.side places the image left or right; the shared
// container registry frames it (heading lives in the text column, not the container header)
export function MediaTextBlock({ block }: { block: Block }) {
  const image = readImage(block.props, "image");
  const heading = readString(block.props, "heading");
  const body = block.props.body;
  const ctaLabel = readString(block.props, "ctaLabel");
  const ctaHref = readString(block.props, "ctaHref"); // top-level link → resolved URL
  const goalEvent = readString(block.props, "goalEvent");
  const side = readString(block.props, "side") ?? "left";
  if (!image && !heading && !lexicalHasContent(body)) return null;
  const Container = getContainer(readString(block.props, "container"));

  const media = image ? (
    <CmsImage
      image={image}
      alt={image.alt || heading || ""}
      sizes="(min-width: 768px) 50vw, 100vw"
      className="w-full rounded-2xl border border-border object-cover"
    />
  ) : null;

  const text = (
    <div className="min-w-0">
      {heading && <h2 className="text-2xl font-extrabold tracking-tight text-foreground">{heading}</h2>}
      {lexicalHasContent(body) && (
        <div className={`text-base leading-relaxed text-muted-foreground ${heading ? "mt-3" : ""}`}>
          <LexicalRichText value={body} />
        </div>
      )}
      {ctaLabel && ctaHref && (
        <GoalLink
          href={ctaHref}
          goalEvent={goalEvent}
          className="mt-5 inline-flex h-11 items-center gap-2 rounded-md bg-primary px-6 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
        >
          {ctaLabel}
        </GoalLink>
      )}
    </div>
  );

  return Container({
    children: (
      <div className="grid items-center gap-8 md:grid-cols-2">
        {side === "right" ? (
          <>
            {text}
            {media}
          </>
        ) : (
          <>
            {media}
            {text}
          </>
        )}
      </div>
    ),
  });
}
