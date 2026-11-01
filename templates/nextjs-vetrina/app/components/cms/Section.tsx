import { Link } from "@/app/components/LocaleLink";
import { ArrowRightIcon } from "lucide-react";

// The header's right side belongs to whatever drives the content - a carousel puts its arrows
// there. The link is a real button under the content instead, where you land after reading it
export function CmsSection({
  heading,
  href,
  hrefLabel,
  actions,
  children,
}: {
  heading?: string;
  href?: string;
  hrefLabel?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
        {(heading || actions) && (
          <div className="mb-6 flex items-end justify-between gap-4">
            <h2 className="text-3xl font-extrabold tracking-tight">{heading}</h2>
            {actions}
          </div>
        )}
        {children}
        {href && hrefLabel && (
          <div className="mt-8 flex justify-center">
            <Link
              href={href}
              className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-foreground/25 px-5 text-sm font-semibold transition hover:border-foreground"
            >
              {hrefLabel}
              <ArrowRightIcon className="h-4 w-4" aria-hidden="true" />
            </Link>
          </div>
        )}
      </div>
    </section>
  );
}
