import { fmtVital, RATING_LABEL, VITAL_LABEL, vitalColor, vitalRating, vitalScore, type VitalMetric } from './vitals'

// VitalGauge is a Lighthouse-style ring gauge for one Core Web Vital: a colored arc (green/amber/red by rating) with
// the p75 value in the center and the metric + rating beneath
export function VitalGauge({ metric, value, samples }: { metric: VitalMetric; value: number; samples?: number }) {
  const score = vitalScore(metric, value)
  const color = vitalColor(metric, value)
  const r = 42
  const c = 2 * Math.PI * r
  return (
    <div className="flex flex-col items-center gap-1.5 text-center">
      <div className="relative">
        <svg width="104" height="104" viewBox="0 0 104 104" className="-rotate-90">
          <circle cx="52" cy="52" r={r} fill="none" strokeWidth="9" className="stroke-muted" />
          <circle
            cx="52"
            cy="52"
            r={r}
            fill="none"
            stroke={color}
            strokeWidth="9"
            strokeLinecap="round"
            strokeDasharray={c}
            strokeDashoffset={c * (1 - score)}
            style={{ transition: 'stroke-dashoffset 600ms ease' }}
          />
        </svg>
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-lg font-semibold tabular-nums" style={{ color }}>
            {fmtVital(metric, value)}
          </span>
        </div>
      </div>
      <div>
        <p className="text-sm font-semibold">{metric}</p>
        <p className="text-[11px] leading-tight text-muted-foreground">{VITAL_LABEL[metric]}</p>
        <p className="text-[11px] font-medium" style={{ color }}>
          {RATING_LABEL[vitalRating(metric, value)]}
        </p>
        {samples != null && <p className="text-[10px] text-muted-foreground">p75 · {samples.toLocaleString()} samples</p>}
      </div>
    </div>
  )
}
