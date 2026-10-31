import { Link } from "@/app/components/LocaleLink";

import type { Block } from "./blocks";
import { readImage, readString, readItems } from "./blocks";
import { CmsImage } from "./CmsImage";

const COLS: Record<number, string> = {
  2: "grid-cols-2",
  3: "grid-cols-2 sm:grid-cols-3",
  4: "grid-cols-2 sm:grid-cols-4",
};

export function GalleryBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const columns = Number(block.props.columns) || 3;
  const items = readItems(block.props, "items").filter((it) => readImage(it, "url"));
  if (items.length === 0) return null;

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
        {heading && (
          <h2 className="mb-6 text-2xl font-extrabold tracking-tight text-foreground">{heading}</h2>
        )}
        <div className={`grid gap-3 ${COLS[columns] ?? COLS[3]}`}>
          {items.map((it, i) => {
            const image = readImage(it, "url")!;
            const alt = readString(it, "alt") ?? image.alt;
            const href = readString(it, "href");
            const img = (
              <CmsImage
                image={image}
                alt={alt}
                sizes="(min-width: 640px) 25vw, 50vw"
                className="aspect-square w-full rounded-lg border border-border object-cover transition group-hover:opacity-90"
              />
            );
            return href ? (
              <Link key={i} href={href} className="group block">
                {img}
              </Link>
            ) : (
              <div key={i} className="group">
                {img}
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
