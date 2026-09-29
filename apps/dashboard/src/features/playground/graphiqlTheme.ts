// GraphiQL themes itself apart from the dashboard (it follows the OS scheme), so anything drawn inside it takes GraphiQL's variables - the dashboard's tokens put dark text on its dark theme
export const GQ = {
  muted: 'text-[hsla(var(--color-neutral),var(--alpha-secondary))]',
  faint: 'text-[hsla(var(--color-neutral),var(--alpha-tertiary))]',
  strong: 'text-[hsl(var(--color-neutral))]',
  border: 'border-[hsla(var(--color-neutral),var(--alpha-background-heavy))]',
  hover: 'hover:bg-[hsla(var(--color-neutral),var(--alpha-background-light))]',
  selected: 'bg-[hsla(var(--color-primary),var(--alpha-background-medium))]',
  success: 'bg-[hsla(var(--color-success),var(--alpha-background-medium))] text-[hsl(var(--color-success))]',
  warning: 'bg-[hsla(var(--color-warning),var(--alpha-background-medium))] text-[hsl(var(--color-warning))]',
  error: 'text-[hsl(var(--color-error))]',
  errorBadge: 'bg-[hsla(var(--color-error),var(--alpha-background-medium))] text-[hsl(var(--color-error))]',
}
