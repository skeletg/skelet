import {
  HeadphonesIcon,
  RotateCcwIcon,
  ShieldCheckIcon,
  TruckIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useTranslations } from "next-intl";

export const VALUE_ICONS: Record<string, LucideIcon> = {
  shipping: TruckIcon,
  returns: RotateCcwIcon,
  secure: ShieldCheckIcon,
  support: HeadphonesIcon,
};

export type ValueItem = { icon: string; title: string; text: string };

// The fallback promises when the CMS block carries none - each names a home.values message
const DEFAULT_VALUE_KEYS = ["shipping", "returns", "secure", "support"] as const;

// Static promises band - hairline-divided columns, no motion
export function ValueStrip({
  items = [],
}: {
  items?: ValueItem[];
}) {
  const t = useTranslations("home.values");
  const shown: ValueItem[] =
    items.length > 0
      ? items
      : DEFAULT_VALUE_KEYS.map((k) => ({
          icon: k,
          title: t(`${k}.title`),
          text: t(`${k}.text`),
        }));
  return (
    <section
      aria-label={t("label")}
      className="border-b border-border bg-background"
    >
      <div className="mx-auto grid w-full max-w-8xl grid-cols-2 gap-y-4 px-6 py-5 md:px-10 lg:grid-cols-4">
        {shown.map((it, i) => {
          const Icon = VALUE_ICONS[it.icon] ?? TruckIcon;
          return (
            <div
              key={it.title}
              className={`flex items-center gap-3 px-2 sm:px-4 ${
                i > 0 ? "lg:border-l lg:border-border" : ""
              } ${i % 2 === 1 ? "border-l border-border lg:border-l" : ""}`}
            >
              <Icon className="h-5 w-5 shrink-0 text-signal" aria-hidden="true" />
              <div className="min-w-0">
                <p className="text-sm font-bold uppercase leading-tight tracking-tight">
                  {it.title}
                </p>
                {it.text && (
                  <p className="truncate text-xs text-muted-foreground">
                    {it.text}
                  </p>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
