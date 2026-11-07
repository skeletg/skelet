import { useEffect, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { BoardsDocument, DiscardBoardDraftDocument, SaveBoardDraftDocument, type BoardFullFragment } from './Boards.gql'
import { fresh, keyed, type DraftReport } from './boardDiff'
import { cleanReport, type BoardReport } from './report'

const KEEP_AFTER_MS = 1000

export type Staged = { name: string; reports: DraftReport[] }

// staging is what a board page opens on: the admin's edits kept from an earlier visit, else the board as saved. A kept
// draft's positions are the board as it was then, so one past the end of the board as it is now reads as added - and
// Save refuses the draft as stale anyway
export function staging(board: BoardFullFragment, saved: BoardReport[], canEdit: boolean): Staged {
  const d = canEdit ? board.draft : null
  if (!d) return { name: board.name, reports: keyed(saved) }
  return {
    name: d.name,
    reports: d.reports.map(({ report, from }) => (from != null && from < saved.length ? { ...cleanReport(report), key: `s${from}`, from } : fresh(cleanReport(report)))),
  }
}

// useKeptDraft keeps an admin's staged board edits on the server about a second after the last one, so a reload or
// another device picks them up - they change nothing anyone else sees, which is why this is the one write that doesn't
// wait for Save (conventions.md). base is the board's updatedAt the edits started from: Save sends it as readAt, so a
// save over someone else's newer one is refused
export function useKeptDraft({ channelId, board, active, staged, dirty, changesKey }: { channelId: string; board?: BoardFullFragment; active: boolean; staged: Staged; dirty: boolean; changesKey: string }) {
  const [base, setBase] = useState('')
  const [restoredAt, setRestoredAt] = useState<string | null>(null)
  const [keep] = useMutation(SaveBoardDraftDocument)
  const [drop] = useMutation(DiscardBoardDraftDocument, { refetchQueries: [BoardsDocument] })

  useEffect(() => {
    if (!active || !board || !base) return
    const t = setTimeout(() => {
      const draft = { name: staged.name, reports: staged.reports.map(({ key: _key, from, ...report }) => ({ from, report })) }
      if (dirty) void keep({ variables: { channelId, id: board.id, draft, baseUpdatedAt: base } }).catch(() => {})
      else if (restoredAt || board.draft) void drop({ variables: { channelId, id: board.id } }).catch(() => {})
    }, KEEP_AFTER_MS)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changesKey, active])

  return {
    base,
    restoredAt,
    // The page adopting a board: what it opens on, and the base its edits start from
    open(b: BoardFullFragment, saved: BoardReport[], canEdit: boolean): Staged {
      const kept = canEdit ? b.draft : null
      setBase(kept ? kept.baseUpdatedAt : b.updatedAt)
      setRestoredAt(kept ? kept.updatedAt : null)
      return staging(b, saved, canEdit)
    },
    // Throwing the kept edits away: the board as saved again
    async discard(b: BoardFullFragment, saved: BoardReport[]): Promise<Staged> {
      await drop({ variables: { channelId, id: b.id } })
      setBase(b.updatedAt)
      setRestoredAt(null)
      return { name: b.name, reports: keyed(saved) }
    },
    // A Save went through: the kept edits are the board now, and later ones start from it
    saved(updatedAt: string) {
      setBase(updatedAt)
      setRestoredAt(null)
    },
  }
}
