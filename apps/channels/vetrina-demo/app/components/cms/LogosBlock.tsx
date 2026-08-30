import { Link } from "@/app/components/LocaleLink";

import type { Block } from "./blocks";
import { readImage, readString, readItems } from "./blocks";
import { CmsImage } from "./CmsImage";
import { getContainer } from "./variants";

export function LogosBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const items = readItems(block.props, "items").filter((it) => readImage(it, "image"));
  if (items.length === 0) return null;
  const Container = getContainer(readString(block.props, "container"));

  return Container({
    heading,
    children: (
      <div className="flex flex-wrap items-center justify-center gap-x-10 gap-y-6">
        {items.map((it, i) => {
          const image = readImage(it, "image")!;
          const href = readString(it, "href");
          const img = (
            <CmsImage
              image={image}
              alt=""
              className="h-8 w-auto object-contain opacity-60 grayscale transition hover:opacity-100 hover:grayscale-0 sm:h-10"
            />
          );
          return href ? (
            <Link key={i} href={href} className="block">
              {img}
            </Link>
          ) : (
            <div key={i}>{img}</div>
          );
        })}
      </div>
    ),
  });
}
