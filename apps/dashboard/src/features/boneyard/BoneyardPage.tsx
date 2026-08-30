import { SkeletonRun } from './SkeletonRun'

export function BoneyardPage() {
  return (
    <div className="flex min-h-[70vh] flex-col items-center justify-center gap-6 px-6">
      <SkeletonRun />
      <p className="max-w-md text-center text-xs text-muted-foreground">
        You found the boneyard. Space or ↑ jumps, ↓ ducks - and not every bat flies at head height.
      </p>
    </div>
  )
}
