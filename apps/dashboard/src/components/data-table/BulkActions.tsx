import { useState, type ComponentType, type ReactNode } from 'react'
import type {
  RowData, ColumnFiltersState } from '@/components/data-table/tanstack'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'

type ButtonVariant = 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost'

// A bulk action is declarative: label, optional icon, optional confirm dialog, and the work to run over the
// selected rows - BulkActionsBar owns the confirm/error/loading lifecycle so features don't re-implement it

// `allMatching` means every row the current filters select, which on a server-paged list far exceeds the page
// the browser holds, so the action must act on the criteria rather than on `rows`
export type BulkScope = {
  allMatching: boolean
  total: number
  globalFilter: string
  columnFilters: ColumnFiltersState
}

export type BulkAction<TData extends RowData> = {
  /** Stable key (defaults to label) */
  id?: string
  label: string
  icon?: ComponentType<{ className?: string }>
  variant?: ButtonVariant
  /** When present, clicking opens an AlertDialog before `perform` runs */
  confirm?: {
    title: string | ((rows: TData[]) => string)
    description?: ReactNode | ((rows: TData[]) => ReactNode)
    actionLabel?: string
  }
  /** The work to do - Throw to surface an error - Honour `scope.allMatching` or you silently act on one page */
  perform: (rows: TData[], scope: BulkScope) => Promise<void>
  /** Disable the button for the current selection */
  disabled?: (rows: TData[], scope: BulkScope) => boolean
  /** Hide the button when the user picked every matching row and the action can only take a page */
  supportsAllMatching?: boolean
  /** Called after a successful run, e.g. to clear selection */
  onSuccess?: () => void
}

function resolveText<T>(
  value: string | ReactNode | ((rows: T[]) => string | ReactNode) | undefined,
  rows: T[],
): ReactNode {
  if (typeof value === 'function') return value(rows) as ReactNode
  return value ?? null
}

export function BulkActionsBar<TData extends RowData>({
  rows,
  actions,
  scope,
  onAfterRun,
}: {
  rows: TData[]
  actions: BulkAction<TData>[]
  scope: BulkScope
  /** Called after any action finishes successfully (e.g. clear selection) */
  onAfterRun?: () => void
}) {
  const [running, setRunning] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<BulkAction<TData> | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function perform(action: BulkAction<TData>) {
    const key = action.id ?? action.label
    setRunning(key)
    setError(null)
    try {
      await action.perform(rows, scope)
      setConfirming(null)
      action.onSuccess?.()
      onAfterRun?.()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Action failed.')
    } finally {
      setRunning(null)
    }
  }

  return (
    <>
      <div className="flex items-center gap-2">
        {actions
          .filter((a) => !scope.allMatching || a.supportsAllMatching !== false)
          .map((action) => {
          const key = action.id ?? action.label
          const Icon = action.icon
          const isRunning = running === key
          const isDisabled = !!action.disabled?.(rows, scope) || isRunning
          return (
            <Button
              key={key}
              size="sm"
              variant={action.variant ?? 'default'}
              disabled={isDisabled}
              onClick={() => {
                setError(null)
                if (action.confirm) setConfirming(action)
                else perform(action)
              }}
            >
              {Icon && <Icon className="size-4" />}
              {isRunning ? 'Working…' : action.label}
            </Button>
          )
        })}
      </div>

      <AlertDialog
        open={!!confirming}
        onOpenChange={(v) => {
          if (!v) {
            setConfirming(null)
            setError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirming && resolveText(confirming.confirm?.title, rows)}
            </AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div>
                {confirming && resolveText(confirming.confirm?.description, rows)}
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          {error && (
            <Alert variant="destructive">
              <AlertTitle>Failed</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={!!running}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={!!running}
              onClick={(e) => {
                e.preventDefault()
                if (confirming) perform(confirming)
              }}
            >
              {running ? 'Working…' : confirming?.confirm?.actionLabel ?? confirming?.label ?? 'Confirm'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
