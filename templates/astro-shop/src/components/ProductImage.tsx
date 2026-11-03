type Image = { url: string; altText?: string | null; width?: number | null; height?: number | null };

type Props = {
  name: string | null;
  image?: Image | null;
  // Only the page's LCP image loads eagerly
  priority?: boolean;
  // Shared between the grid card and the product page, so the image morphs across the navigation
  transitionName?: string;
  sizes?: string;
  className?: string;
};

// A product photo over a tinted initial - with no photo, or one that fails to load, the initial shows. The box is the
// same either way, so nothing shifts
export function ProductImage({ name, image, priority = false, transitionName, sizes, className = "" }: Props) {
  const label = name ?? "";
  const hue = [...label].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 7);
  return (
    <div
      className={`relative aspect-4/5 w-full overflow-hidden rounded-2xl bg-[hsl(var(--h)_32%_88%)] dark:bg-[hsl(var(--h)_14%_20%)] ${className}`}
      style={{ "--h": hue, viewTransitionName: transitionName } as React.CSSProperties}
    >
      <span
        aria-hidden="true"
        className="absolute inset-0 grid place-items-center text-6xl font-semibold tracking-tight text-foreground/25 transition-transform duration-500 ease-(--ease-out-soft) group-hover:scale-110"
      >
        {label.charAt(0)}
      </span>
      {image && (
        // A broken image paints its icon and alt text - browsers draw an <img>'s ::before only when it failed, so that
        // covers them with the same tint and initial
        <img
          src={image.url}
          alt={image.altText || label}
          data-initial={label.charAt(0)}
          width={image.width ?? 800}
          height={image.height ?? 1000}
          sizes={sizes}
          loading={priority ? "eager" : "lazy"}
          fetchPriority={priority ? "high" : "auto"}
          decoding="async"
          className="relative h-full w-full bg-inherit object-cover text-transparent transition-transform duration-500 ease-(--ease-out-soft) group-hover:scale-[1.04] before:absolute before:inset-0 before:grid before:place-items-center before:bg-inherit before:text-6xl before:font-semibold before:tracking-tight before:text-foreground/25 before:content-[attr(data-initial)]"
        />
      )}
    </div>
  );
}
