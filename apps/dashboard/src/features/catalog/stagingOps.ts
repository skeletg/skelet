import { randomId } from '@/lib/random-id'

// The queue mechanics every staging module shares. Bundles, grouped members and product options
// each stage edits as ops and collapse them before committing, and each had its own copy of the
// three loops below - which is how ONE coalescing bug ended up shipped three times
//
// The cancellation phase deliberately stays with each module: product options cascade a cancelled
// add to the values underneath it, the other two have nothing to cascade to

/** An unsaved local id - the committer rewrites these to real server ids during apply */
export const TEMP_PREFIX = 'tmp_'

export function newTempId(): string {
  return TEMP_PREFIX + randomId()
}

export function isTemp(id: string): boolean {
  return id.startsWith(TEMP_PREFIX)
}

// Thrown when a mid-sequence mutation fails - carries the unapplied tail (temp refs already resolved) so the section
// can keep just that and a retry resumes instead of re-running committed adds and duplicating rows
export class CommitError<Op = unknown> extends Error {
  remaining: Op[]
  // Temp ids that became real before the failure - anything else still naming them has to follow
  idMap: Map<string, string>
  constructor(cause: unknown, remaining: Op[], idMap: Map<string, string> = new Map()) {
    super(cause instanceof Error ? cause.message : String(cause))
    this.name = 'CommitError'
    this.cause = cause
    this.remaining = remaining
    this.idMap = idMap
  }
}

/**
 * What the collapser needs to know about an op, without knowing the op
 *
 * `entity` keeps two records of different kinds from merging just because they share an id, and
 * `id` is the tempId for an add and the target id for anything else - which is exactly what makes
 * an update fold into the add that created its record
 */
export type OpView = {
  role: 'add' | 'update' | 'delete'
  entity: string
  id: string
}

export type OpMerges<Op> = {
  /** later update wins per field - `acc` is what has been folded so far, NOT the op that started the run */
  updates: (acc: Op, next: Op) => Op
  /** an add that is still local carries its updates inline, so only one create mutation fires */
  intoAdd: (add: Op, update: Op) => Op
}

/**
 * Collapses a queue that has already had its cancelled adds removed:
 *   update(id) -> update(id)   merge fields, first position wins
 *   add(temp)  -> update(temp) fold into the add
 *   update(id) with a delete(id) anywhere   drop the update, the delete wins
 *
 * Order-preserving - a survivor stays at the earliest position it held
 */
export function collapseOps<Op>(ops: Op[], view: (op: Op) => OpView, merge: OpMerges<Op>): Op[] {
  const result = [...ops]
  const sameRecord = (a: OpView, b: OpView) => a.entity === b.entity && a.id === b.id

  for (let i = 0; i < result.length; i++) {
    if (view(result[i]).role !== 'update') continue
    const target = view(result[i])
    for (let j = i + 1; j < result.length; j++) {
      const other = view(result[j])
      if (other.role !== 'update' || !sameRecord(target, other)) continue
      // reads result[i], not the op this run started from - folding into the latter drops any
      // field only a middle update touched
      result[i] = merge.updates(result[i], result[j])
      result.splice(j, 1)
      j--
    }
  }

  for (let i = 0; i < result.length; i++) {
    if (view(result[i]).role !== 'add') continue
    const created = view(result[i])
    for (let j = i + 1; j < result.length; j++) {
      const other = view(result[j])
      if (other.role !== 'update' || !sameRecord(created, other)) continue
      result[i] = merge.intoAdd(result[i], result[j])
      result.splice(j, 1)
      j--
    }
  }

  // The record is going away, so the update would be work the server throws out
  for (let i = result.length - 1; i >= 0; i--) {
    const target = view(result[i])
    if (target.role !== 'update') continue
    const deleted = result.some(
      (o, j) => j !== i && view(o).role === 'delete' && sameRecord(target, view(o)),
    )
    if (deleted) result.splice(i, 1)
  }

  return result
}
