import { useContext } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  ZapIcon,
  MailIcon,
  MessageSquareIcon,
  ClockIcon,
  GitBranchIcon,
  TagIcon,
  WebhookIcon,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { FlowCanvasContext } from './flowCanvasContext'
import { NODE_LABELS, TRIGGER_LABELS, nodeSummary, type FlowNodeData } from './flowTypes'

const ICONS: Record<string, LucideIcon> = {
  trigger: ZapIcon,
  send_email: MailIcon,
  send_sms: MessageSquareIcon,
  wait: ClockIcon,
  split: GitBranchIcon,
  add_segment: TagIcon,
  webhook: WebhookIcon,
}

export function StepIcon({ type, className }: { type: string; className?: string }) {
  const Icon = ICONS[type] ?? ZapIcon
  return <Icon className={className} />
}

function Box({
  id,
  type,
  summary,
  selected,
}: {
  id: string
  type: string
  summary: string
  selected?: boolean
}) {
  const Icon = ICONS[type] ?? ZapIcon
  const flag = useContext(FlowCanvasContext).flags.get(id)
  return (
    <div
      className={cn(
        'relative w-48 rounded-lg border bg-card px-3 py-2 shadow-sm transition-shadow',
        selected ? 'ring-2 ring-primary' : 'hover:shadow',
      )}
    >
      {flag ? (
        <span
          role="img"
          aria-label={flag === 'error' ? 'Needs fixing' : 'Worth a look'}
          className={cn('absolute -right-1 -top-1 size-3 rounded-full border-2 border-card', flag === 'error' ? 'bg-destructive' : 'bg-warning')}
        />
      ) : null}
      <div className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
        <Icon className="size-3.5 text-primary" />
        {NODE_LABELS[type] ?? type}
      </div>
      <div className="mt-0.5 truncate text-[11px] text-muted-foreground">{summary}</div>
    </div>
  )
}

export function TriggerNode({ id, data, selected }: NodeProps) {
  const d = data as FlowNodeData
  const label = d.label ? TRIGGER_LABELS[d.label] ?? d.label : 'choose a trigger'
  return (
    <div className="relative">
      <Box id={id} type="trigger" summary={label} selected={selected} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

// LeafBody is the shared shell for the single-in/single-out node types
function LeafBody({ id, type, data, selected }: { id: string; type: string; data: unknown; selected?: boolean }) {
  const { names } = useContext(FlowCanvasContext)
  return (
    <div className="relative">
      <Handle type="target" position={Position.Top} />
      <Box id={id} type={type} summary={nodeSummary(type, data as FlowNodeData, names)} selected={selected} />
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

export function EmailNode({ id, data, selected }: NodeProps) {
  return <LeafBody id={id} type="send_email" data={data} selected={selected} />
}
export function SMSNode({ id, data, selected }: NodeProps) {
  return <LeafBody id={id} type="send_sms" data={data} selected={selected} />
}
export function WaitNode({ id, data, selected }: NodeProps) {
  return <LeafBody id={id} type="wait" data={data} selected={selected} />
}
export function SegmentNode({ id, data, selected }: NodeProps) {
  return <LeafBody id={id} type="add_segment" data={data} selected={selected} />
}
export function WebhookNode({ id, data, selected }: NodeProps) {
  return <LeafBody id={id} type="webhook" data={data} selected={selected} />
}

export function SplitNode({ id, data, selected }: NodeProps) {
  return (
    <div className="relative">
      <Handle type="target" position={Position.Top} />
      <Box id={id} type="split" summary={nodeSummary('split', data as FlowNodeData)} selected={selected} />
      <Handle id="yes" type="source" position={Position.Bottom} style={{ left: '28%' }} />
      <Handle id="no" type="source" position={Position.Bottom} style={{ left: '72%' }} />
      <span className="absolute -bottom-4 left-[20%] text-[9px] font-medium text-success">yes</span>
      <span className="absolute -bottom-4 left-[66%] text-[9px] font-medium text-destructive">no</span>
    </div>
  )
}
