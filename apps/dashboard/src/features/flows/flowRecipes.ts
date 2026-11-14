import { GAP_Y, type Link, type Step } from './flowGraph'
import { defaultNodeData, type FlowNodeData } from './flowTypes'

// Every recipe is a marketing flow, so picking one never stands in for a receipt or a sign-in link

const HOUR = 3600
const DAY = 86400

type RecipeStep = { type: 'send_email' | 'wait'; data?: FlowNodeData }

export type FlowRecipe = { id: string; name: string; description: string; trigger: string; steps: RecipeStep[] }

const email: RecipeStep = { type: 'send_email' }
const wait = (seconds: number): RecipeStep => ({ type: 'wait', data: { delaySeconds: seconds } })

export const FLOW_RECIPES: FlowRecipe[] = [
  {
    id: 'welcome',
    name: 'Welcome series',
    description: 'Greet a new customer, then follow up three days later.',
    trigger: 'customer_registered',
    steps: [email, wait(3 * DAY), email],
  },
  {
    id: 'cart',
    name: 'Abandoned cart',
    description: 'Two reminders about a cart left behind, a day apart.',
    trigger: 'cart_abandoned',
    steps: [wait(HOUR), email, wait(DAY), email],
  },
  {
    id: 'browse',
    name: 'Browse follow-up',
    description: 'Write to someone who looked at a product and left.',
    trigger: 'browse_abandoned',
    steps: [wait(2 * HOUR), email],
  },
  {
    id: 'back-in-stock',
    name: 'Back in stock',
    description: 'Tell a customer the product they asked about is available again.',
    trigger: 'back_in_stock',
    steps: [email],
  },
  {
    id: 'signup',
    name: 'Newsletter welcome',
    description: 'Welcome a new subscriber, then introduce the store two days later.',
    trigger: 'form_submitted',
    steps: [email, wait(2 * DAY), email],
  },
]

export function buildRecipe(recipe: FlowRecipe, trigger: Step, id: () => string): { steps: Step[]; links: Link[] } {
  const steps: Step[] = recipe.steps.map((s, i) => ({
    id: id(),
    type: s.type,
    position: { x: trigger.position.x, y: trigger.position.y + (i + 1) * GAP_Y },
    data: { ...defaultNodeData(s.type), ...s.data },
  }))
  const links: Link[] = steps.map((s, i) => ({ id: id(), source: i === 0 ? trigger.id : steps[i - 1]!.id, target: s.id }))
  return { steps, links }
}
