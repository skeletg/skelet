import { Fragment } from 'react'
import { StepIcon } from './FlowNodes'
import type { FlowRecipe } from './flowRecipes'
import { TRIGGER_LABELS } from './flowTypes'

export function FlowStart({ recipes, onPick }: { recipes: FlowRecipe[]; onPick: (r: FlowRecipe) => void }) {
  return (
    <section aria-label="Start from a recipe" className="mx-auto w-[min(48rem,calc(100%-1.5rem))] rounded-xl border bg-card p-4 shadow-sm">
      <div className="mb-3 text-center">
        <h2 className="text-sm font-medium">Start from a recipe</h2>
        <p className="text-xs text-muted-foreground">Every step can be changed afterwards - or click a step on the left to build from the trigger alone.</p>
      </div>
      <div className="grid grid-cols-1 gap-2 @min-[560px]/canvas:grid-cols-2 @min-[760px]/canvas:grid-cols-3">
        {recipes.map((r) => (
          <button
            key={r.id}
            type="button"
            onClick={() => onPick(r)}
            className="grid content-start gap-1.5 rounded-md border bg-muted/40 p-2.5 text-left transition-colors hover:border-primary/50 hover:bg-accent/40"
          >
            <span aria-hidden className="flex items-center">
              {['trigger', ...r.steps.map((s) => s.type)].map((type, i) => (
                <Fragment key={i}>
                  {i > 0 ? <span className="h-px w-2.5 bg-foreground/25" /> : null}
                  <span className="grid size-6 place-items-center rounded border bg-background text-muted-foreground">
                    <StepIcon type={type} className="size-3" />
                  </span>
                </Fragment>
              ))}
            </span>
            <span className="text-sm font-medium text-foreground">{r.name}</span>
            <span className="text-xs text-muted-foreground">{r.description}</span>
            <span className="text-[11px] text-muted-foreground/80">Starts on: {TRIGGER_LABELS[r.trigger] ?? r.trigger}</span>
          </button>
        ))}
      </div>
    </section>
  )
}
