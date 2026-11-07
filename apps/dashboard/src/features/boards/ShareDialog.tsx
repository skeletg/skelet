import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import type { BoardShareLevel } from '@/graphql/schema.types'
import { ACCESS_LABEL, LEVELS } from './access'
import { BoardShareTargetsDocument, BoardsDocument, SetBoardSharesDocument, type BoardFullFragment, type BoardShareTargetsQuery } from './Boards.gql'

type Share = { adminId?: string | null; roleId?: string | null; name: string; level: BoardShareLevel }
type Target = BoardShareTargetsQuery['boardShareTargets'][number]

const idOf = (s: Pick<Share, 'adminId' | 'roleId'>) => s.adminId ?? s.roleId ?? ''

// What a share would actually show the person - a grant can come later, so this warns rather than refuses
export function shareWarning(t: Target | undefined, shop: string): string | null {
  if (!t) return null
  if (!t.seesBoards) return `${t.kind === 'role' ? 'This role has' : 'They have'} no access to boards on ${shop}, so ${t.kind === 'role' ? 'its members' : 'they'} won't see it.`
  if (!t.seesData) return `${t.kind === 'role' ? 'This role has' : 'They have'} no customer analytics on ${shop}: the board's layout shows, its numbers don't.`
  return null
}

// ShareDialog edits who a board is shared with, applied by its own Save - a dialog with an explicit action, so it
// isn't staged on the page (conventions.md). It's mounted only while open, so it always starts from the board's shares
export function ShareDialog({ channelId, shop, board, onClose }: { channelId: string; shop: string; board: BoardFullFragment; onClose: () => void }) {
  const [shares, setShares] = useState<Share[]>(() => board.shares.map((s) => ({ adminId: s.adminId, roleId: s.roleId, name: s.name, level: s.level })))
  const [pick, setPick] = useState('')
  const [pickLevel, setPickLevel] = useState<BoardShareLevel>('VIEW')
  const { data } = useQuery(BoardShareTargetsDocument, { variables: { channelId } })
  const [save, { loading, error }] = useMutation(SetBoardSharesDocument, { refetchQueries: [BoardsDocument] })
  const targets = data?.boardShareTargets ?? []
  const target = (id: string) => targets.find((t) => t.id === id)
  const free = targets.filter((t) => t.id !== board.owner?.id && !shares.some((s) => idOf(s) === t.id))

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Share {board.name}</DialogTitle>
          <DialogDescription>Only the people and roles below see this board. Each sees its numbers with their own access to the shop's data.</DialogDescription>
        </DialogHeader>
        <ul className="divide-y rounded-md border text-sm">
          {shares.length === 0 && <li className="px-3 py-3 text-muted-foreground">Not shared yet - only you can see it.</li>}
          {shares.map((s) => {
            const warn = shareWarning(target(idOf(s)), shop)
            return (
              <li key={idOf(s)} className="px-3 py-2">
                <div className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate">
                    {s.name} <span className="text-xs text-muted-foreground">{s.roleId ? 'role' : 'admin'}</span>
                  </span>
                  <select
                    aria-label={`What ${s.name} can do`}
                    value={s.level}
                    onChange={(e) => setShares((xs) => xs.map((x) => (idOf(x) === idOf(s) ? { ...x, level: e.target.value as BoardShareLevel } : x)))}
                    className="rounded border bg-background px-2 py-1 text-xs"
                  >
                    {LEVELS.map((l) => (
                      <option key={l} value={l}>
                        {ACCESS_LABEL[l]}
                      </option>
                    ))}
                  </select>
                  <Button size="icon" variant="ghost" className="size-7" aria-label={`Stop sharing with ${s.name}`} onClick={() => setShares((xs) => xs.filter((x) => idOf(x) !== idOf(s)))}>
                    <XIcon className="size-3.5" />
                  </Button>
                </div>
                {warn && <p className="mt-1 text-xs text-warning">{warn}</p>}
              </li>
            )
          })}
        </ul>
        <div className="flex items-end gap-2">
          <div className="min-w-0 flex-1 space-y-1">
            <Label htmlFor="share-with">Share with</Label>
            <select id="share-with" value={pick} onChange={(e) => setPick(e.target.value)} className="w-full rounded border bg-background px-2 py-1.5 text-sm">
              <option value="">Choose a role or an admin…</option>
              {(['role', 'admin'] as const).map((kind) => (
                <optgroup key={kind} label={kind === 'role' ? 'Roles' : 'Admins'}>
                  {free
                    .filter((t) => t.kind === kind)
                    .map((t) => (
                      <option key={t.id} value={t.id}>
                        {t.name}
                      </option>
                    ))}
                </optgroup>
              ))}
            </select>
          </div>
          <select aria-label="What they can do" value={pickLevel} onChange={(e) => setPickLevel(e.target.value as BoardShareLevel)} className="rounded border bg-background px-2 py-1.5 text-sm">
            {LEVELS.map((l) => (
              <option key={l} value={l}>
                {ACCESS_LABEL[l]}
              </option>
            ))}
          </select>
          <Button
            variant="outline"
            disabled={!pick}
            onClick={() => {
              const t = target(pick)
              if (!t) return
              setShares((xs) => [...xs, { adminId: t.kind === 'admin' ? t.id : null, roleId: t.kind === 'role' ? t.id : null, name: t.name, level: pickLevel }])
              setPick('')
            }}
          >
            Add
          </Button>
        </div>
        {pick && shareWarning(target(pick), shop) && <p className="text-xs text-warning">{shareWarning(target(pick), shop)}</p>}
        {error && <p className="text-sm text-destructive">{error.message}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={loading}
            onClick={async () => {
              const res = await save({
                variables: { channelId, id: board.id, shares: shares.map((s) => ({ adminId: s.adminId ?? null, roleId: s.roleId ?? null, level: s.level })) },
              }).catch(() => null)
              if (res?.data) onClose()
            }}
          >
            Save sharing
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
