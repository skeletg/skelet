import { useState } from 'react'
import { gql } from '@apollo/client'
import { useMutation } from '@apollo/client/react'
import { Loader2Icon } from 'lucide-react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const GENERATE_PROMOTION_CODES = gql`
  mutation GeneratePromotionCodes(
    $ruleId: ID!
    $pattern: String!
    $count: Int!
    $prefix: String
    $suffix: String
  ) {
    generatePromotionCodes(
      ruleId: $ruleId
      pattern: $pattern
      count: $count
      prefix: $prefix
      suffix: $suffix
    ) {
      id
      code
    }
  }
`

// CodeGeneratorDialog wraps the backend's pattern-based bulk code
// generator - Pattern grammar:
//   [A] uppercase letter   [N] digit   [*] alphanumeric
// Anything outside square brackets is literal - Prefix + suffix wrap
// the whole pattern - Example: pattern="WELCOME-[N][N][N]" yields
// "WELCOME-481", "WELCOME-237", etc.
export function CodeGeneratorDialog({
  ruleId,
  open,
  onOpenChange,
  onGenerated,
}: {
  ruleId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onGenerated?: () => void
}) {
  const [pattern, setPattern] = useState('[A][A][N][N][N][N]')
  const [count, setCount] = useState(10)
  const [prefix, setPrefix] = useState('')
  const [suffix, setSuffix] = useState('')
  const [error, setError] = useState<string | null>(null)

  const [generate, { loading }] = useMutation(GENERATE_PROMOTION_CODES)

  async function onGenerate() {
    setError(null)
    try {
      await generate({
        variables: {
          ruleId,
          pattern,
          count,
          prefix: prefix || null,
          suffix: suffix || null,
        },
      })
      onGenerated?.()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Generate failed')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Generate codes</DialogTitle>
          <DialogDescription>
            Pattern grammar: <code>[A]</code> uppercase letter, <code>[N]</code> digit,{' '}
            <code>[*]</code> alphanumeric. Everything else is literal.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 py-2">
          <div className="grid gap-1.5">
            <Label htmlFor="pattern">Pattern</Label>
            <Input
              id="pattern"
              value={pattern}
              onChange={(e) => setPattern(e.target.value)}
              className="font-mono"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="count">Count</Label>
            <Input
              id="count"
              type="number"
              min={1}
              max={10000}
              value={count}
              onChange={(e) => setCount(parseInt(e.target.value || '0', 10))}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="prefix">Prefix (optional)</Label>
              <Input id="prefix" value={prefix} onChange={(e) => setPrefix(e.target.value)} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="suffix">Suffix (optional)</Label>
              <Input id="suffix" value={suffix} onChange={(e) => setSuffix(e.target.value)} />
            </div>
          </div>
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={onGenerate} disabled={loading || count <= 0}>
            {loading ? <Loader2Icon className="size-4 animate-spin" /> : null}
            Generate
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
