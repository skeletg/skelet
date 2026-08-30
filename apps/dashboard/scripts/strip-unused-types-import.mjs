// near-operation-file emits `import type * as Types` into every .gql.ts, but only operations touching an
// enum, input or custom scalar use it - codegen's afterAllFileWrite hook, so noUnusedLocals can stay on
import { readFile, writeFile } from 'node:fs/promises'

const IMPORT_LINE = /^import type \* as Types from '[^']+';?$/

async function strip(file) {
  const src = await readFile(file, 'utf8')
  const lines = src.split('\n')
  const at = lines.findIndex((l) => IMPORT_LINE.test(l.trim()))
  if (at === -1) return false
  // Word-boundary so a `MyTypes.` or `Types` inside a string can't read as a use
  const used = lines.some((l, i) => i !== at && /\bTypes\./.test(l))
  if (used) return false
  lines.splice(at, 1)
  await writeFile(file, lines.join('\n'))
  return true
}

const files = process.argv.slice(2).filter((f) => f.endsWith('.gql.ts'))
const stripped = await Promise.all(files.map(strip))
const n = stripped.filter(Boolean).length
if (n > 0) console.log(`stripped unused Types import from ${n} file(s)`)
