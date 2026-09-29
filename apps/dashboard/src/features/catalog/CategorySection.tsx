import type { ReactNode } from 'react'

// One titled block on the category detail pane - split out so the sections that live in their own
// files render the same heading as the ones inside CategoriesPage
export function Section({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description?: string
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="grid gap-4 px-6 py-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-lg font-semibold tracking-tight">
            <span className="mr-1.5 text-primary">#</span>{title}
          </h3>
          {description ? (
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
      </div>
      <div className="grid gap-4">{children}</div>
    </section>
  )
}
