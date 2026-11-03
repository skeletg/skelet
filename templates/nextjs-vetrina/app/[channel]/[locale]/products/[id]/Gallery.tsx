"use client";

import { useState, ViewTransition } from "react";
import { useTranslations } from "next-intl";

import { Modal } from "@/app/components/Modal";
import { SnapButton, SnapTrack, useSnapCarousel } from "@/app/components/SnapCarousel";
import type { ProductMedia } from "@skelet/storefront-next/catalog/read";
import { mediaSrcSet, mediaThumb } from "@skelet/storefront-core/media-thumb";
import { productImageTransitionName } from "@/app/lib/view-transition";

type Props = {
  media: ProductMedia[];
  // Single-image fallback for products with no `media` rows
  fallback: ProductMedia | null;
  alt: string;
  // Pairs the first slide with the grid card the shopper clicked, so the thumbnail morphs into the
  // hero instead of one image vanishing and another appearing
  morphFromProductId?: string;
};

export function ProductGallery({ media, fallback, alt, morphFromProductId }: Props) {
  const items: ProductMedia[] =
    media.length > 0 ? media : fallback ? [fallback] : [];
  const tc = useTranslations("common");

  if (items.length === 0) {
    return (
      <div className="flex aspect-square w-full items-center justify-center overflow-hidden rounded-2xl border border-border/60 bg-muted">
        <span className="text-sm text-muted-foreground">{tc("noImage")}</span>
      </div>
    );
  }

  const mediaKey = items.map((m) => m.id).join(",");

  return (
    <ProductGalleryInner
      key={mediaKey}
      items={items}
      alt={alt}
      morphFromProductId={morphFromProductId}
    />
  );
}

function ProductGalleryInner({
  items,
  alt,
  morphFromProductId,
}: {
  items: ProductMedia[];
  alt: string;
  morphFromProductId?: string;
}) {
  const t = useTranslations("product.gallery");
  const [lightboxOpen, setLightboxOpen] = useState(false);
  const count = items.length;
  const { ref, index, scrollTo, prev, next } = useSnapCarousel(count);

  return (
    <div>
      <div className="relative">
        <SnapTrack trackRef={ref} className="w-full rounded-2xl">
          {items.map((m, i) => (
            // Only the first slide is the shopper's landing image, so only it pairs with the card -
            // naming the whole track would try to morph a thumbnail into a multi-slide scroller
            <MaybeMorph
              key={m.id}
              name={i === 0 ? morphFromProductId : undefined}
            >
            <button
              type="button"
              onClick={() => setLightboxOpen(true)}
              aria-label={t("open")}
              className="group block aspect-square w-full cursor-zoom-in overflow-hidden rounded-2xl border border-border/60 bg-muted"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={mediaThumb(m.url, 560, m.width)}
                srcSet={mediaSrcSet(m.url, m.width)}
                sizes="(min-width: 1024px) 560px, 100vw"
                alt={m.altText || alt}
                width={560}
                height={560}
                loading={i === 0 ? "eager" : "lazy"}
                fetchPriority={i === 0 ? "high" : "auto"}
                decoding="async"
                className="h-full w-full object-cover"
              />
            </button>
            </MaybeMorph>
          ))}
        </SnapTrack>

        {count > 1 && (
          <>
            <SnapButton dir="prev" onClick={prev} className="left-3" />
            <SnapButton dir="next" onClick={next} className="right-3" />
            <div className="pointer-events-none absolute bottom-3 right-3 rounded-md bg-black/60 px-2.5 py-1 text-xs font-medium text-white backdrop-blur-sm">
              {index + 1} / {count}
            </div>
          </>
        )}
      </div>

      {count > 1 && (
        <ul className="mt-3 grid grid-cols-5 gap-2.5">
          {items.map((m, i) => {
            const picked = i === index;
            return (
              <li key={m.id}>
                <button
                  type="button"
                  onClick={() => scrollTo(i)}
                  aria-label={t("show", { index: i + 1 })}
                  aria-pressed={picked}
                  className={[
                    "block aspect-square w-full overflow-hidden rounded-lg transition",
                    picked
                      ? "ring-2 ring-foreground ring-offset-2 ring-offset-background"
                      : "opacity-60 ring-1 ring-border hover:opacity-100",
                  ].join(" ")}
                >
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={mediaThumb(m.url, 160, m.width)}
                    alt={m.altText || alt}
                    width={80}
                    height={80}
                    loading="lazy"
                    decoding="async"
                    className="h-full w-full object-cover"
                  />
                </button>
              </li>
            );
          })}
        </ul>
      )}

      <Modal
        open={lightboxOpen}
        onOpenChange={setLightboxOpen}
        label={t("viewer")}
        className="lightbox"
      >
        {lightboxOpen && (
          <Lightbox
            items={items}
            startAt={index}
            alt={alt}
            onClose={() => setLightboxOpen(false)}
          />
        )}
      </Modal>
    </div>
  );
}

function Lightbox({
  items,
  startAt,
  alt,
  onClose,
}: {
  items: ProductMedia[];
  startAt: number;
  alt: string;
  onClose: () => void;
}) {
  const count = items.length;
  const { ref, index, prev, next } = useSnapCarousel(count, startAt);
  const tc = useTranslations("common");

  return (
    <div className="relative w-full max-w-5xl">
      <SnapTrack trackRef={ref}>
        {items.map((m) => (
          <div key={m.id} className="flex justify-center">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={m.url}
              alt={m.altText || alt}
              className="max-h-[85vh] w-auto max-w-full object-contain"
            />
          </div>
        ))}
      </SnapTrack>
      {count > 1 && (
        <>
          <SnapButton
            dir="prev"
            onClick={prev}
            className="left-4 border-none bg-white/10 text-white hover:bg-white/20"
          />
          <SnapButton
            dir="next"
            onClick={next}
            className="right-4 border-none bg-white/10 text-white hover:bg-white/20"
          />
          <div className="pointer-events-none absolute bottom-3 left-1/2 -translate-x-1/2 rounded-md bg-white/10 px-3 py-1 text-sm text-white">
            {index + 1} / {count}
          </div>
        </>
      )}
      <button
        type="button"
        onClick={onClose}
        className="absolute right-4 top-4 flex size-9 items-center justify-center rounded-md bg-white/10 text-white transition hover:bg-white/20"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          strokeLinecap="round"
          className="size-4"
          aria-hidden="true"
        >
          <path d="M18 6 6 18M6 6l12 12" />
        </svg>
        <span className="sr-only">{tc("close")}</span>
      </button>
    </div>
  );
}

// A ViewTransition only where a name was supplied - React needs the name at the element it should
// pair, and wrapping the unnamed slides too would give them all "auto" names
function MaybeMorph({
  name,
  children,
}: {
  name?: string;
  children: React.ReactNode;
}) {
  if (!name) return children;
  return (
    <ViewTransition name={productImageTransitionName(name)} share="morph" default="none">
      {children}
    </ViewTransition>
  );
}
