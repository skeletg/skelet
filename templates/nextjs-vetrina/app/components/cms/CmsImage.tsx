import type { ImageValue } from "./blocks";

// Plain <img> with intrinsic dimensions, lazy/eager loading and focal-point positioning Not next/image - the media host is env-driven, so
// remotePatterns would be brittle This is the seam where a real srcset pipeline goes (see mediaSrcSet in lib/media-thumb)
export function CmsImage({
  image,
  className,
  sizes,
  alt,
  eager,
}: {
  image: ImageValue;
  className?: string;
  sizes?: string;
  /** Overrides image.alt (e.g. a per-item alt, or "" for a decorative image). */
  alt?: string;
  /** Above-the-fold image → load eagerly instead of lazily. */
  eager?: boolean;
}) {
  const style =
    image.focalX != null && image.focalY != null
      ? { objectPosition: `${image.focalX * 100}% ${image.focalY * 100}%` }
      : undefined;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={image.src}
      alt={alt ?? image.alt}
      width={image.width}
      height={image.height}
      sizes={sizes}
      loading={eager ? "eager" : "lazy"}
      decoding="async"
      className={className}
      style={style}
    />
  );
}
