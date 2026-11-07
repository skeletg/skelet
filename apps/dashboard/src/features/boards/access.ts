import type { BoardAccess, BoardShareLevel } from '@/graphql/schema.types'

// Each access includes the ones before it
const RANK: Record<BoardAccess, number> = { VIEW: 1, EDIT: 2, OWN: 3 }
export const can = (has: BoardAccess | undefined, need: BoardAccess) => !!has && RANK[has] >= RANK[need]

export const ACCESS_LABEL: Record<BoardAccess, string> = { VIEW: 'Can view', EDIT: 'Can edit', OWN: 'Owner' }
export const LEVELS: BoardShareLevel[] = ['VIEW', 'EDIT']
// What an access lets someone do with a board, as a verb: "view", "edit", "own"
export const verb = (a: BoardAccess) => a.toLowerCase()
