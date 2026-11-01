import type { ReactNode } from "react";

// A band's eyebrow, title and optional intro (the children) - centred over a grid, or left beside an image
type Props = {
  eyebrow: string;
  title: string;
  id?: string;
  center?: boolean;
  children?: ReactNode;
};

export function SectionHeading({ eyebrow, title, id, center = false, children }: Props) {
  return (
    <div className={center ? "mx-auto max-w-2xl text-center" : "max-w-xl"}>
      <p className={`eyebrow flex items-center gap-3 text-accent before:h-px before:w-8 before:bg-current${center ? " justify-center" : ""}`}>{eyebrow}</p>
      <h2 id={id} className="mt-4 text-3xl font-semibold leading-[1.1] tracking-tight text-balance sm:text-[2.75rem]">
        {title}
      </h2>
      {children != null && <div className="mt-5 text-base leading-relaxed text-pretty text-muted-foreground">{children}</div>}
    </div>
  );
}
