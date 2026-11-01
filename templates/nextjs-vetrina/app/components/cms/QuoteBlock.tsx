import type { Block } from "./blocks";
import { readImage, readString } from "./blocks";
import { CmsImage } from "./CmsImage";

export function QuoteBlock({ block }: { block: Block }) {
  const quote = readString(block.props, "quote");
  const author = readString(block.props, "author");
  const role = readString(block.props, "role");
  const avatar = readImage(block.props, "avatar");
  const variant = readString(block.props, "variant") ?? "card";
  if (!quote) return null;

  const Attribution = (author || role) && (
    <figcaption className="mt-4 flex items-center gap-3">
      {avatar && (
        <CmsImage
          image={avatar}
          alt={author ?? avatar.alt}
          className="size-10 rounded-full border border-border object-cover"
        />
      )}
      <span className="text-sm">
        {author && <span className="block font-medium text-foreground">{author}</span>}
        {role && <span className="block text-muted-foreground">{role}</span>}
      </span>
    </figcaption>
  );

  if (variant === "large") {
    return (
      <section className="border-b border-border">
        <figure className="mx-auto w-full max-w-3xl px-6 py-16 text-center">
          <blockquote className="text-2xl font-medium leading-snug tracking-tight text-foreground sm:text-3xl">
            “{quote}”
          </blockquote>
          {Attribution && <div className="mt-6 flex justify-center">{Attribution}</div>}
        </figure>
      </section>
    );
  }

  if (variant === "minimal") {
    return (
      <section className="border-b border-border">
        <figure className="mx-auto w-full max-w-3xl px-6 py-12">
          <blockquote className="border-l-4 border-foreground/70 pl-5 text-lg leading-relaxed text-foreground">
            {quote}
          </blockquote>
          {Attribution}
        </figure>
      </section>
    );
  }

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-3xl px-6 py-12">
        <figure className="glass rounded-2xl p-8">
          <blockquote className="text-lg leading-relaxed text-foreground">“{quote}”</blockquote>
          {Attribution}
        </figure>
      </div>
    </section>
  );
}
