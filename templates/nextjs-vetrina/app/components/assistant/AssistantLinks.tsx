import { ArrowRightIcon, FileTextIcon, SearchIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import type { AssistantLink } from "@skelet/storefront-core/assistant/types";
import { joinBasePathWithFilters } from "@skelet/storefront-core/filter-path";
import { Link } from "@/app/components/LocaleLink";

// The address in the storefront's own filter grammar, the same one the listing pages read
export function assistantHref(link: AssistantLink): string {
  return joinBasePathWithFilters(link.path, link.filters, link.sort, link.query ? new URLSearchParams({ q: link.query }).toString() : "");
}

// Buttons to where the reply sends the shopper: a narrowed listing with its product count, or a page
export function AssistantLinks({ links }: { links: AssistantLink[] }) {
  const t = useTranslations("assistant.links");
  return (
    <div className="grid gap-2">
      {links.map((link, i) => (
        <Link
          key={i}
          href={assistantHref(link)}
          className="group flex items-center gap-3 rounded-lg border border-border px-3 py-2.5 transition hover:border-foreground/30 hover:bg-muted"
        >
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-signal">
            {link.count != null ? <SearchIcon className="size-4" aria-hidden="true" /> : <FileTextIcon className="size-4" aria-hidden="true" />}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">{link.label}</span>
            {link.count != null ? <span className="block text-xs text-muted-foreground">{t("count", { count: link.count })}</span> : null}
          </span>
          <ArrowRightIcon className="size-4 shrink-0 text-muted-foreground transition group-hover:translate-x-0.5" aria-hidden="true" />
        </Link>
      ))}
    </div>
  );
}
