import { useState } from 'react'
import { Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'
import { cn } from '@/lib/utils'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

// The backend's default-locale provider falls back to "en" when no channel
// is flagged default - mirror it so the guard below matches the server
const FALLBACK_DEFAULT_LOCALE = 'en'

// Shared per-locale strip for content editors: a pill per stored locale plus the destructive "Remove translation"
// action for the locale being edited - Editors take that locale from the header scope
export function TranslationLocales({
  locale,
  locales,
  entityLabel,
  canManage,
  defaultLocaleLocked = false,
  staged = false,
  onRemove,
  className,
}: {
  /** Locale being edited - empty when the header scope resolves to none */
  locale: string
  /** Locales that already have a stored content row */
  locales: string[]
  /** Used in the confirm copy, e.g. "post" -> "Remove the de translation of this post?" */
  entityLabel: string
  canManage: boolean
  /** Backend refuses to drop the default locale while translations exist (posts + catalog categories) */
  defaultLocaleLocked?: boolean
  /** onRemove only stages the removal for the page's Save - the confirm says so instead of warning it can't be undone */
  staged?: boolean
  /** Runs the delete mutation - resolve only once the refetch has landed */
  onRemove: () => Promise<void>
  className?: string
}) {
  const { globalDefaultLocale } = useConfigScope()
  const [confirming, setConfirming] = useState(false)

  const existing = [...new Set(locales)].sort()
  const hasContent = !!locale && existing.includes(locale)
  const defaultLocale = globalDefaultLocale || FALLBACK_DEFAULT_LOCALE

  // Why the remove action is off - shown next to the disabled button so
  // the operator doesn't have to guess
  let blockedReason: string | null = null
  if (!locale) blockedReason = 'Pick a locale in the header first'
  else if (!hasContent) blockedReason = `Nothing stored for ${locale} yet`
  else if (defaultLocaleLocked && locale === defaultLocale && existing.length > 1)
    blockedReason = `${locale} is the default locale - remove the other translations first`

  return (
    <div
      className={cn(
        'flex flex-wrap items-center justify-between gap-3 rounded-md border px-3 py-2',
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="mr-1 text-xs font-medium text-muted-foreground">Translations</span>
        {existing.map((code) => (
          <span
            key={code}
            className={
              code === locale
                ? 'inline-flex h-6 items-center rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground'
                : 'inline-flex h-6 items-center rounded-md border px-2 text-xs text-muted-foreground'
            }
          >
            {code}
          </span>
        ))}
        {locale && !hasContent ? (
          <span className="inline-flex h-6 items-center rounded-md border border-dashed px-2 text-xs text-muted-foreground">
            new: {locale}
          </span>
        ) : null}
        {!existing.length && !locale ? (
          <span className="text-xs text-muted-foreground">None yet</span>
        ) : null}
      </div>

      {canManage ? (
        <div className="flex flex-wrap items-center justify-end gap-2">
          {blockedReason ? (
            <span className="text-xs text-muted-foreground">{blockedReason}</span>
          ) : null}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="text-destructive hover:bg-destructive/10 hover:text-destructive"
            onClick={() => setConfirming(true)}
            disabled={!!blockedReason}
          >
            <Trash2Icon className="size-4" />
            Remove translation
          </Button>
        </div>
      ) : null}

      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={<>Remove the {locale} translation?</>}
        description={
          <>
            This deletes the {locale} content of this {entityLabel} - other locales stay as they
            are. {staged ? 'It is removed when you save the page.' : 'This action cannot be undone.'}
          </>
        }
        confirmLabel={staged ? 'Stage remove' : 'Remove translation'}
        busyLabel={staged ? 'Staging…' : 'Removing…'}
        destructive
        onConfirm={onRemove}
      />
    </div>
  )
}
