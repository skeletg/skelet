// One pill link into a blog taxonomy archive

import { Link } from "@/app/components/LocaleLink";
import type { ReactNode } from "react";

export function BlogTaxonomyChip({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className="inline-block rounded-md bg-muted px-2.5 py-1 text-muted-foreground hover:bg-accent hover:text-foreground"
    >
      {children}
    </Link>
  );
}
