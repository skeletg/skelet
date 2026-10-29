// Staging layer for the Grouped structure editor - mirrors bundleStaging.ts but with only one entity type - Members carry tempIds for
// session additions, resolved to real ids as adds complete so later updates and deletes write to the right row

import { CommitError, collapseOps, isTemp, type OpView } from '@/features/catalog/stagingOps'
import type { SectionDiffEntry } from './productSaveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'

export type MemberProduct = {
  id: string
  sku?: string | null
  name?: string | null
  kind: string
  defaultPrice: number
  defaultCurrencyCode: string
}

export type GroupedMember = {
  id: string
  member: MemberProduct
  defaultQuantity: number
  sortOrder: number
}

export { CommitError, TEMP_PREFIX, isTemp, newTempId } from '@/features/catalog/stagingOps'


export type AddMemberOp = {
  kind: 'add-member'
  tempId: string
  product: MemberProduct
  defaultQuantity: number
  sortOrder: number
}

export type UpdateMemberOp = {
  kind: 'update-member'
  id: string // tempId or real
  defaultQuantity?: number
  sortOrder?: number
}

export type RemoveMemberOp = {
  kind: 'remove-member'
  id: string
}

export type PendingOp = AddMemberOp | UpdateMemberOp | RemoveMemberOp


// reduceOps simplifies the queue so the diff count tracks user intent
//   add(tempId) → remove(tempId)  → drop both
//   add(tempId) → update(tempId)  → merge update fields into add
//   update(id)  → update(id)      → merge fields into first update
//   update(id)  → remove(id)      → drop the update
// What the shared collapser needs to know about a member op - see stagingOps
const view = (op: PendingOp): OpView => ({
  role: op.kind === 'add-member' ? 'add' : op.kind === 'update-member' ? 'update' : 'delete',
  entity: 'member',
  id: op.kind === 'add-member' ? op.tempId : op.id,
})

export function reduceOps(ops: PendingOp[]): PendingOp[] {
  // Drop add+remove pairs by tempId - nothing cascades under a member, so this stays local
  let result: PendingOp[] = [...ops]
  result = result.filter((op, idx) => {
    if (op.kind !== 'add-member') return true
    const tempId = op.tempId
    return !result.some((o, i) => i > idx && o.kind === 'remove-member' && o.id === tempId)
  })
  result = result.filter((op) => {
    if (op.kind !== 'remove-member') return true
    if (!isTemp(op.id)) return true
    return !ops.some((o) => o.kind === 'add-member' && o.tempId === op.id)
  })

  return collapseOps(result, view, {
    updates: (acc, next) =>
      ({ ...(acc as UpdateMemberOp), ...(next as UpdateMemberOp), id: (acc as UpdateMemberOp).id }),
    intoAdd: (add, upd) => {
      const a = add as AddMemberOp
      const u = upd as UpdateMemberOp
      return { ...a, defaultQuantity: u.defaultQuantity ?? a.defaultQuantity, sortOrder: u.sortOrder ?? a.sortOrder }
    },
  })
}


// A derived view with every pending op materialised - the UI renders from this, so a staged
// "Add member" shows immediately even though no mutation has run
export function applyOps(
  original: readonly GroupedMember[],
  ops: readonly PendingOp[],
): GroupedMember[] {
  let members: GroupedMember[] = original.map((m) => ({ ...m }))

  for (const op of ops) {
    switch (op.kind) {
      case 'add-member':
        members.push({
          id: op.tempId,
          member: op.product,
          defaultQuantity: op.defaultQuantity,
          sortOrder: op.sortOrder,
        })
        break
      case 'update-member':
        members = members.map((m) => {
          if (m.id !== op.id) return m
          const next = { ...m }
          if (op.defaultQuantity !== undefined) next.defaultQuantity = op.defaultQuantity
          if (op.sortOrder !== undefined) next.sortOrder = op.sortOrder
          return next
        })
        break
      case 'remove-member':
        members = members.filter((m) => m.id !== op.id)
        break
    }
  }
  return members
}


export function opsToDiff(
  original: readonly GroupedMember[],
  ops: readonly PendingOp[],
): SectionDiffEntry[] {
  const byId = new Map(original.map((m) => [m.id, m]))
  return ops.flatMap((op): SectionDiffEntry[] => {
    switch (op.kind) {
      case 'add-member':
        return [added(`Member · ${op.product.name ?? op.product.sku ?? ''}`, `qty ${op.defaultQuantity}`)]
      case 'update-member': {
        const prev = byId.get(op.id)
        const label = `Member · ${prev?.member.name ?? prev?.member.sku ?? op.id}`
        const rows: SectionDiffEntry[] = []
        if (op.defaultQuantity !== undefined) {
          const e = changed(`${label} · qty`, prev?.defaultQuantity, op.defaultQuantity)
          if (e) rows.push(e)
        }
        if (op.sortOrder !== undefined) {
          const e = changed(`${label} · order`, prev?.sortOrder, op.sortOrder)
          if (e) rows.push(e)
        }
        return rows
      }
      case 'remove-member': {
        const prev = byId.get(op.id)
        return [removed(`Member · ${prev?.member.name ?? prev?.member.sku ?? op.id}`)]
      }
    }
  })
}


export type Committers = {
  addMember: (op: AddMemberOp & { parentId: string }) => Promise<string>
  updateMember: (op: UpdateMemberOp) => Promise<void>
  removeMember: (op: { id: string }) => Promise<void>
}

export async function commitOps(
  ops: readonly PendingOp[],
  parentId: string,
  committers: Committers,
): Promise<{ idMap: Map<string, string> }> {
  const idMap = new Map<string, string>()
  const resolve = (id: string) => idMap.get(id) ?? id

  for (let i = 0; i < ops.length; i++) {
    const op = ops[i]
    try {
      switch (op.kind) {
        case 'add-member': {
          const realId = await committers.addMember({ ...op, parentId })
          idMap.set(op.tempId, realId)
          break
        }
        case 'update-member': {
          const id = resolve(op.id)
          await committers.updateMember({ ...op, id })
          break
        }
        case 'remove-member': {
          const id = resolve(op.id)
          if (isTemp(id)) break
          await committers.removeMember({ id })
          break
        }
      }
    } catch (err) {
      const remaining = ops.slice(i).map((o): PendingOp => (o.kind === 'add-member' ? o : { ...o, id: resolve(o.id) }))
      throw new CommitError(err, remaining, idMap)
    }
  }

  return { idMap }
}
