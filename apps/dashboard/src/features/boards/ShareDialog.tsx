import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import type { BoardShareLevel } from '@/graphql/schema.types'
import { ACCESS_LABEL, LEVELS } from './access'
import {
  BoardDocument,
  BoardShareTargetsDocument,
  BoardsDocument,
  SetBoardSharesDocument,
  TransferBoardDocument,
  type BoardFullFragment,
  type BoardShareTargetsQuery,
} from './Boards.gql'

// OWN on a row is the admin the board is handed to on Save - it isn't a share, so it never goes to setBoardShares
type Level = BoardShareLevel | 'OWN'
type Share = { adminId?: string | null; roleId?: string | null; name: string; level: Level }
type Target = BoardShareTargetsQuery['boardShareTargets'][number]

const idOf = (s: Pick<Share, 'adminId' | 'roleId'>) => s.adminId ?? s.roleId ?? ''

// What a share would actually show the person - a grant can come later, so this warns rather than refuses
export function shareWarning(t: Target | undefined, shop: string): string | null {
  if (!t) return null
  if (!t.seesBoards) return `${t.kind === 'role' ? 'This role has' : 'They have'} no access to boards on ${shop}, so ${t.kind === 'role' ? 'its members' : 'they'} won't see it.`
  if (!t.seesData) return `${t.kind === 'role' ? 'This role has' : 'They have'} no customer analytics on ${shop}: the board's layout shows, its numbers don't.`
  return null
}

function LevelSelect({ label, value, owner, onChange, className }: { label: string; value: Level; owner: boolean; onChange: (l: Level) => void; className: string }) {
  return (
    <select aria-label={label} value={value} onChange={(e) => onChange(e.target.value as Level)} className={className}>
      {(owner ? [...LEVELS, 'OWN' as const] : LEVELS).map((l) => (
        <option key={l} value={l}>
          {l === 'OWN' ? 'Make owner' : ACCESS_LABEL[l]}
        </option>
      ))}
    </select>
  )
}

// ShareDialog is everything about who has a board: who it's shared with and at what level, who owns it, and - for
// someone who manages boards - whether it's a shop board everyone on the shop sees. Applied by its own Save, a dialog
// with an explicit action, so it isn't staged on the page (conventions.md). Mounted only while open, so it always
// starts from the board as it is
export function ShareDialog({ channelId, shop, board, canManage, onClose }: { channelId: string; shop: string; board: BoardFullFragment; canManage: boolean; onClose: () => void }) {
  const wasShop = !board.owner
  const [shares, setShares] = useState<Share[]>(() => board.shares.map((s) => ({ adminId: s.adminId, roleId: s.roleId, name: s.name, level: s.level })))
  const [everyone, setEveryone] = useState(wasShop)
  const [pick, setPick] = useState('')
  const [pickLevel, setPickLevel] = useState<Level>(wasShop ? 'OWN' : 'VIEW')
  const { data } = useQuery(BoardShareTargetsDocument, { variables: { channelId } })
  const [setBoardShares, sharing] = useMutation(SetBoardSharesDocument, { refetchQueries: [BoardsDocument] })
  const [transfer, transferring] = useMutation(TransferBoardDocument, { refetchQueries: [BoardsDocument, BoardDocument], awaitRefetchQueries: true })
  const [saving, setSaving] = useState(false)
  const error = sharing.error ?? transferring.error

  const targets = data?.boardShareTargets ?? []
  const target = (id: string) => targets.find((t) => t.id === id)
  const free = targets.filter((t) => t.id !== board.owner?.id && !shares.some((s) => idOf(s) === t.id))
  const newOwner = shares.find((s) => s.level === 'OWN')
  // A shop board made private needs someone to own it first - they decide who else sees it
  const needsOwner = !everyone && wasShop

  // One owner at a time: making someone the owner puts anyone picked before back to editing
  const withLevel = (xs: Share[], id: string, level: Level) => xs.map((x) => (idOf(x) === id ? { ...x, level } : level === 'OWN' && x.level === 'OWN' ? { ...x, level: 'EDIT' as const } : x))

  async function save() {
    setSaving(true)
    try {
      if (everyone) {
        if (!wasShop) await transfer({ variables: { channelId, id: board.id, ownerId: null } })
      } else if (wasShop) {
        if (newOwner) await transfer({ variables: { channelId, id: board.id, ownerId: newOwner.adminId } })
      } else {
        // Shares first, while the caller still owns it; then the hand-over, which keeps them on as an editor
        const rest = shares.filter((s) => s.level !== 'OWN').map((s) => ({ adminId: s.adminId ?? null, roleId: s.roleId ?? null, level: s.level as BoardShareLevel }))
        await setBoardShares({ variables: { channelId, id: board.id, shares: rest } })
        if (newOwner) await transfer({ variables: { channelId, id: board.id, ownerId: newOwner.adminId } })
      }
      onClose()
    } catch {
      // the mutation's error is shown below
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Share {board.name}</DialogTitle>
          <DialogDescription>Each person sees its numbers with their own access to the shop's data.</DialogDescription>
        </DialogHeader>
        {!everyone && (
          <>
            <ul className="divide-y rounded-md border text-sm">
              {board.owner && (
                <li className="flex items-center gap-2 px-3 py-2">
                  <span className="min-w-0 flex-1 truncate">{board.owner.name}</span>
                  <span className="text-xs text-muted-foreground">{newOwner ? 'Owner - can edit after Save' : ACCESS_LABEL.OWN}</span>
                </li>
              )}
              {!board.owner && shares.length === 0 && <li className="px-3 py-3 text-muted-foreground">Pick who owns it - they decide who else sees it.</li>}
              {board.owner && shares.length === 0 && <li className="px-3 py-3 text-muted-foreground">Not shared yet - only its owner can see it.</li>}
              {shares.map((s) => {
                const warn = shareWarning(target(idOf(s)), shop)
                return (
                  <li key={idOf(s)} className="px-3 py-2">
                    <div className="flex items-center gap-2">
                      <span className="min-w-0 flex-1 truncate">
                        {s.name} <span className="text-xs text-muted-foreground">{s.roleId ? 'role' : 'admin'}</span>
                      </span>
                      <LevelSelect
                        label={`What ${s.name} can do`}
                        value={s.level}
                        owner={!s.roleId}
                        onChange={(l) => setShares((xs) => withLevel(xs, idOf(s), l))}
                        className="rounded border bg-background px-2 py-1 text-xs"
                      />
                      <Button size="icon" variant="ghost" className="size-7" aria-label={`Stop sharing with ${s.name}`} onClick={() => setShares((xs) => xs.filter((x) => idOf(x) !== idOf(s)))}>
                        <XIcon className="size-3.5" />
                      </Button>
                    </div>
                    {s.level === 'OWN' && <p className="mt-1 text-xs text-muted-foreground">Becomes the owner when you save{board.owner ? `, and ${board.owner.name} can still edit it` : ''}.</p>}
                    {warn && <p className="mt-1 text-xs text-warning">{warn}</p>}
                  </li>
                )
              })}
            </ul>
            {!(needsOwner && newOwner) && (
              <div className="flex items-end gap-2">
                <div className="min-w-0 flex-1 space-y-1">
                  <Label htmlFor="share-with">{needsOwner ? 'Owner' : 'Share with'}</Label>
                  <select id="share-with" value={pick} onChange={(e) => setPick(e.target.value)} className="w-full rounded border bg-background px-2 py-1.5 text-sm">
                    <option value="">{needsOwner ? 'Choose an admin…' : 'Choose a role or an admin…'}</option>
                    {(needsOwner ? (['admin'] as const) : (['role', 'admin'] as const)).map((kind) => (
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
                {!needsOwner && (
                  <LevelSelect label="What they can do" value={pickLevel} owner={target(pick)?.kind === 'admin'} onChange={setPickLevel} className="rounded border bg-background px-2 py-1.5 text-sm" />
                )}
                <Button
                  variant="outline"
                  disabled={!pick}
                  onClick={() => {
                    const t = target(pick)
                    if (!t) return
                    const level: Level = needsOwner ? 'OWN' : pickLevel === 'OWN' && t.kind !== 'admin' ? 'VIEW' : pickLevel
                    setShares((xs) => withLevel([...xs, { adminId: t.kind === 'admin' ? t.id : null, roleId: t.kind === 'role' ? t.id : null, name: t.name, level: 'VIEW' }], t.id, level))
                    setPick('')
                    setPickLevel('VIEW')
                  }}
                >
                  Add
                </Button>
              </div>
            )}
            {pick && shareWarning(target(pick), shop) && <p className="text-xs text-warning">{shareWarning(target(pick), shop)}</p>}
          </>
        )}
        {(canManage || wasShop) && (
          <div className="space-y-1">
            <Label htmlFor="general-access">General access</Label>
            <select
              id="general-access"
              value={everyone ? 'shop' : 'people'}
              disabled={!canManage}
              onChange={(e) => setEveryone(e.target.value === 'shop')}
              className="w-full rounded border bg-background px-2 py-1.5 text-sm"
            >
              <option value="people">Only the people above</option>
              <option value="shop">Everyone on {shop} - a shop board</option>
            </select>
            {everyone && !wasShop && <p className="text-xs text-muted-foreground">Everyone on {shop} with access to boards will see it, anyone who manages boards here runs it, and its shares go.</p>}
          </div>
        )}
        {error && <p className="text-sm text-destructive">{error.message}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={saving || (needsOwner && !newOwner)} onClick={() => void save()}>
            Save sharing
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
