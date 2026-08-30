import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  ZapIcon,
  MailIcon,
  MessageSquareIcon,
  ClockIcon,
  GitBranchIcon,
  TagIcon,
  WebhookIcon,
  SatelliteDishIcon,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { NODE_LABELS, TRIGGER_LABELS, nodeSummary, type FlowNodeData } from './flowTypes'

const ICONS: Record<string, LucideIcon> = {
  trigger: ZapIcon,
  send_email: MailIcon,
  send_sms: MessageSquareIcon,
  wait: ClockIcon,
  split: GitBranchIcon,
  add_segment: TagIcon,
  webhook: WebhookIcon,
  remote_action: SatelliteDishIcon,
}

function Box({
  type,
  summary,
  selected,
}: {
  type: string
  summary: string
  selected?: boolean
}) {
  const Icon = ICONS[type] ?? ZapIcon
  return (
    <div
      className={cn(
        'w-48 rounded-lg border bg-card px-3 py-2 shadow-sm transition-shadow',
        selected ? 'ring-2 ring-primary' : 'hover:shadow',
      )}
    >
      <div className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
        <Icon className="size-3.5 text-primary" />
        {NODE_LABELS[type] ?? type}
      </div>
      <div className="mt-0.5 truncate text-[11px] text-muted-foreground">{summary}</div>
    </div>
  )
}

export function TriggerNode({ data, selected }: NodeProps) {
  const d = data as FlowNodeData
  const label = d.label ? TRIGGER_LABELS[d.label] ?? d.label : 'choose a trigger'
  return (
    <div className="relative">
      <Box type="trigger" summary={label} selected={selected} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

// LeafBody is the shared shell for the single-in/single-out node types
function LeafBody({ type, data, selected }: { type: string; data: unknown; selected?: boolean }) {
  return (
    <div className="relative">
      <Handle type="target" position={Position.Top} />
      <Box type={type} summary={nodeSummary(type, data as FlowNodeData)} selected={selected} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

export function EmailNode({ data, selected }: NodeProps) {
  return <LeafBody type="send_email" data={data} selected={selected} />
}
export function SMSNode({ data, selected }: NodeProps) {
  return <LeafBody type="send_sms" data={data} selected={selected} />
}
export function WaitNode({ data, selected }: NodeProps) {
  return <LeafBody type="wait" data={data} selected={selected} />
}
export function SegmentNode({ data, selected }: NodeProps) {
  return <LeafBody type="add_segment" data={data} selected={selected} />
}
export function WebhookNode({ data, selected }: NodeProps) {
  return <LeafBody type="webhook" data={data} selected={selected} />
}
export function RemoteActionNode({ data, selected }: NodeProps) {
  return <LeafBody type="remote_action" data={data} selected={selected} />
}

export function SplitNode({ data, selected }: NodeProps) {
  return (
    <div className="relative">
      <Handle type="target" position={Position.Top} />
      <Box type="split" summary={nodeSummary('split', data as FlowNodeData)} selected={selected} />
      <Handle id="yes" type="source" position={Position.Bottom} style={{ left: '28%' }} />
      <Handle id="no" type="source" position={Position.Bottom} style={{ left: '72%' }} />
      <span className="absolute -bottom-4 left-[20%] text-[9px] font-medium text-green-600">yes</span>
      <span className="absolute -bottom-4 left-[66%] text-[9px] font-medium text-rose-600">no</span>
    </div>
  )
}
