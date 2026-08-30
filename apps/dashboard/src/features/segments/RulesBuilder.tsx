import { useQuery } from '@apollo/client/react'
import { PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { CustomerGroupsDocument } from '@/features/customers/CustomerGroups.gql'
import {
  CONSENT_KINDS,
  KIND_LABELS,
  LIFECYCLE_STAGES,
  NUMERIC_OPS,
  TRAIT_FIELDS,
  defaultCondition,
  type RuleCondition,
  type Rules,
} from './rules'

// Condition-tree editor for dynamic segments - Controlled - the parent stages
// the value into the global Save
export function RulesBuilder({
  value,
  onChange,
}: {
  value: Rules
  onChange: (next: Rules) => void
}) {
  const { data: groupsData } = useQuery(CustomerGroupsDocument)
  const groups = groupsData?.customerGroups ?? []

  const setCondition = (i: number, next: RuleCondition) =>
    onChange({ ...value, conditions: value.conditions.map((c, j) => (j === i ? next : c)) })
  const removeCondition = (i: number) =>
    onChange({ ...value, conditions: value.conditions.filter((_, j) => j !== i) })
  const addCondition = () =>
    onChange({ ...value, conditions: [...value.conditions, defaultCondition('trait')] })

  const num = (v: string) => (v.trim() === '' ? 0 : Number(v))

  return (
    <div className="grid gap-3">
      <div className="flex items-center gap-2">
        <Label className="text-sm font-normal text-muted-foreground">Customer matches</Label>
        <Select
          value={value.logic ?? 'and'}
          onValueChange={(v) => onChange({ ...value, logic: v as Rules['logic'] })}
        >
          <SelectTrigger className="h-8 w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="and">all conditions</SelectItem>
            <SelectItem value="or">any condition</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-2">
        {value.conditions.map((c, i) => (
          <div
            key={i}
            className="flex flex-wrap items-center gap-2 rounded-md border border-l-4 border-l-primary/40 px-3 py-2"
          >
            <Select
              value={c.kind}
              onValueChange={(kind) => setCondition(i, defaultCondition(kind as RuleCondition['kind']))}
            >
              <SelectTrigger className="h-8 w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(KIND_LABELS).map(([k, label]) => (
                  <SelectItem key={k} value={k}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {c.kind === 'trait' && (
              <>
                <Select value={c.field} onValueChange={(field) => setCondition(i, { ...c, field })}>
                  <SelectTrigger className="h-8 w-56">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {TRAIT_FIELDS.map((f) => (
                      <SelectItem key={f.value} value={f.value}>
                        {f.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <OpSelect value={c.op} onChange={(op) => setCondition(i, { ...c, op })} />
                <Input
                  type="number"
                  className="h-8 w-28"
                  value={c.value ?? 0}
                  onChange={(e) => setCondition(i, { ...c, value: num(e.target.value) })}
                />
              </>
            )}

            {c.kind === 'lifecycle' && (
              <>
                <Select value={c.op} onValueChange={(op) => setCondition(i, { ...c, op })}>
                  <SelectTrigger className="h-8 w-24">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="eq">is</SelectItem>
                    <SelectItem value="neq">is not</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={c.stage} onValueChange={(stage) => setCondition(i, { ...c, stage })}>
                  <SelectTrigger className="h-8 w-36">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LIFECYCLE_STAGES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </>
            )}

            {c.kind === 'order_count' && (
              <>
                <OpSelect
                  value={c.op}
                  onChange={(op) => setCondition(i, { ...c, op })}
                  ops={['gte', 'lte', 'eq']}
                />
                <Input
                  type="number"
                  min="0"
                  className="h-8 w-24"
                  value={c.value ?? 0}
                  onChange={(e) => setCondition(i, { ...c, value: num(e.target.value) })}
                />
                <DaysInput value={c.days ?? 0} onChange={(days) => setCondition(i, { ...c, days })} />
              </>
            )}

            {c.kind === 'total_spent_cents' && (
              <>
                <OpSelect
                  value={c.op}
                  onChange={(op) => setCondition(i, { ...c, op })}
                  ops={['gte', 'lte']}
                />
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  className="h-8 w-28"
                  value={(c.value ?? 0) / 100}
                  onChange={(e) => setCondition(i, { ...c, value: Math.round(num(e.target.value) * 100) })}
                />
                <span className="text-xs text-muted-foreground">in store currency</span>
                <DaysInput value={c.days ?? 0} onChange={(days) => setCondition(i, { ...c, days })} />
              </>
            )}

            {c.kind === 'last_order_days' && (
              <>
                <Select value={c.op} onValueChange={(op) => setCondition(i, { ...c, op })}>
                  <SelectTrigger className="h-8 w-36">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="lte">within last</SelectItem>
                    <SelectItem value="gte">at least</SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  type="number"
                  min="0"
                  className="h-8 w-24"
                  value={c.value ?? 0}
                  onChange={(e) => setCondition(i, { ...c, value: num(e.target.value) })}
                />
                <span className="text-xs text-muted-foreground">days ago</span>
              </>
            )}

            {c.kind === 'consent' && (
              <>
                <Select
                  value={c.channel}
                  onValueChange={(channel) => setCondition(i, { ...c, channel })}
                >
                  <SelectTrigger className="h-8 w-44">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CONSENT_KINDS.map((k) => (
                      <SelectItem key={k.value} value={k.value}>
                        {k.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={c.granted ?? false}
                    onCheckedChange={(granted) => setCondition(i, { ...c, granted })}
                  />
                  <span className="text-xs text-muted-foreground">
                    {c.granted ? 'opted in' : 'not opted in'}
                  </span>
                </div>
              </>
            )}

            {c.kind === 'group' && (
              <Select value={c.slug || undefined} onValueChange={(slug) => setCondition(i, { ...c, slug })}>
                <SelectTrigger className="h-8 w-56">
                  <SelectValue placeholder="Pick a group" />
                </SelectTrigger>
                <SelectContent>
                  {groups.map((g) => (
                    <SelectItem key={g.slug} value={g.slug}>
                      {g.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}

            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="ml-auto size-7"
              aria-label="Remove condition"
              onClick={() => removeCondition(i)}
            >
              <XIcon className="size-3.5" />
            </Button>
          </div>
        ))}
      </div>

      <div>
        <Button type="button" variant="outline" size="sm" onClick={addCondition}>
          <PlusIcon className="size-4" />
          Add condition
        </Button>
      </div>
    </div>
  )
}

function OpSelect({
  value,
  onChange,
  ops,
}: {
  value?: string
  onChange: (op: string) => void
  ops?: string[]
}) {
  const options = ops ? NUMERIC_OPS.filter((o) => ops.includes(o.value)) : NUMERIC_OPS
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 w-20">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function DaysInput({ value, onChange }: { value: number; onChange: (days: number) => void }) {
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-xs text-muted-foreground">in last</span>
      <Input
        type="number"
        min="0"
        className="h-8 w-20"
        value={value}
        onChange={(e) => onChange(e.target.value.trim() === '' ? 0 : Number(e.target.value))}
      />
      <span className="text-xs text-muted-foreground">days (0 = all time)</span>
    </div>
  )
}
