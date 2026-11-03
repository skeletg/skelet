import { Link } from "@/app/components/LocaleLink";

import type { Block } from "./blocks";
import { readImage, readString, readItems } from "./blocks";
import { CmsImage } from "./CmsImage";
import { getContainer } from "./variants";

const COLS: Record<string, string> = {
  two: "sm:grid-cols-2",
  three: "sm:grid-cols-3",
  four: "sm:grid-cols-2 lg:grid-cols-4",
};

export function FeatureGridBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const cols = readString(block.props, "cols") ?? "three";
  const items = readItems(block.props, "items").filter((it) => readString(it, "title"));
  if (items.length === 0) return null;
  const Container = getContainer(readString(block.props, "container"));

  return Container({
    heading,
    children: (
      <div className={`grid grid-cols-1 gap-5 ${COLS[cols] ?? COLS.three}`}>
        {items.map((it, i) => {
          const image = readImage(it, "image");
          const title = readString(it, "title")!;
          const text = readString(it, "text");
          const href = readString(it, "href");
          const inner = (
            <>
              {image && <CmsImage image={image} alt="" className="size-10 object-contain" />}
              <h3 className="mt-3 text-base font-medium text-foreground">{title}</h3>
              {text && <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{text}</p>}
            </>
          );
          return href ? (
            <Link key={i} href={href} className="glass glass-hover block rounded-2xl p-5 transition">
              {inner}
            </Link>
          ) : (
            <div key={i} className="rounded-2xl border border-border p-5">
              {inner}
            </div>
          );
        })}
      </div>
    ),
  });
}
