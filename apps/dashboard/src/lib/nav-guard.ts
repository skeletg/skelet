import { useEffect } from 'react'
import { useBlocker } from 'react-router-dom'
import { confirmLeavePage, hasUnsavedChanges, urlOwningTab } from '@/lib/unsaved-changes'

// Navigating inside a tab swaps the page out and takes its staged edits with it, so this asks
// first - it needs react-router's data router, which is why App.tsx mounts RouterProvider
//
// Blocking is the whole point: on a link click the router consults us BEFORE touching state, and
// on back/forward it rewinds history without ever committing, so the dirty page is never unmounted
// and its drafts survive the refusal

// Pane and tab plumbing navigates on its own (switching, splitting, moving a tab between panes),
// and none of it unmounts a page - the workspace provider arms this one-shot before those
let allowOnce = false

export function allowNextNavigation(): void {
  allowOnce = true
  // Blockers are consulted synchronously inside navigate(), so anything still armed a microtask
  // later belongs to a navigation that never happened - dropping it keeps the flag from leaking
  queueMicrotask(() => {
    allowOnce = false
  })
}

// A page that navigates from inside an action it just committed - saving, or a delete the admin
// confirmed - is still holding drafts, but they no longer stand for unsaved work - asking
// "discard changes?" on the way out of a save is pure noise
let committing = 0

export async function duringCommit<T>(run: () => T | Promise<T>): Promise<Awaited<T>> {
  committing++
  try {
    return await run()
  } finally {
    committing--
  }
}

function consumeBypass(): boolean {
  if (!allowOnce) return false
  allowOnce = false
  return true
}

export function useNavGuard(): void {
  const blocker = useBlocker(({ currentLocation, nextLocation }) => {
    if (committing > 0 || consumeBypass()) return false
    // Filters, sorting and pagination ride the search string and keep the same page mounted
    if (currentLocation.pathname === nextLocation.pathname) return false
    return hasUnsavedChanges(urlOwningTab())
  })

  useEffect(() => {
    if (blocker.state !== 'blocked') return
    if (confirmLeavePage()) blocker.proceed?.()
    else blocker.reset?.()
  }, [blocker])
}
