export type ForceNode = {
  key: string
  ring: number // depth from the homepage, with unreachable parked on an outer ring
  x: number
  y: number
  vx: number
  vy: number
  radius: number
  fixed: boolean
  seedAngle: number
}

export type ForceEdge = {
  source: number
  target: number
  length: number
}

export type SimulationOptions = {
  // Radius per ring index - sized by ring population upstream, so a ring holding a thousand pages gets the
  // circumference to hold them instead of being blown apart by repulsion
  ringRadii: number[]
  repulsion?: number
  linkStrength?: number
  radialStrength?: number
  angularStrength?: number
}

const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5))
const MIN_SEPARATION_SQ = 400
const MAX_SPEED = 55

// Deterministic jitter - a seeded hash beats Math.random so a re-render lands the same layout
function jitter(i: number): number {
  const x = Math.sin(i * 12.9898) * 43758.5453
  return x - Math.floor(x) - 0.5
}

export class Simulation {
  readonly nodes: ForceNode[]
  readonly edges: ForceEdge[]
  alpha = 1

  private readonly ringRadii: number[]
  private readonly repulsion: number
  private readonly linkStrength: number
  private readonly radialStrength: number
  private readonly angularStrength: number
  private readonly cell: number
  private grid = new Map<number, number[]>()

  constructor(nodes: ForceNode[], edges: ForceEdge[], opts: SimulationOptions) {
    this.nodes = nodes
    this.edges = edges
    this.ringRadii = opts.ringRadii
    this.repulsion = opts.repulsion ?? 1400
    this.linkStrength = opts.linkStrength ?? 0.035
    this.radialStrength = opts.radialStrength ?? 0.08
    this.angularStrength = opts.angularStrength ?? 0.05
    this.cell = 90
    this.seed()
  }

  // Rings first, phyllotaxis angle second - the layout is already readable on frame one and the forces only refine it
  private seed() {
    const perRing = new Map<number, number>()
    this.nodes.forEach((n, i) => {
      const seen = perRing.get(n.ring) ?? 0
      perRing.set(n.ring, seen + 1)
      const angle = seen * GOLDEN_ANGLE + n.ring * 0.7
      n.seedAngle = angle
      const r = this.radiusFor(n.ring) + jitter(i) * 24
      n.x = Math.cos(angle) * r
      n.y = Math.sin(angle) * r
      n.vx = 0
      n.vy = 0
    })
  }

  radiusFor(ring: number): number {
    return this.ringRadii[Math.min(ring, this.ringRadii.length - 1)] ?? ring * 190
  }

  reheat() {
    this.alpha = 1
  }

  private rebuildGrid() {
    this.grid.clear()
    for (let i = 0; i < this.nodes.length; i++) {
      const n = this.nodes[i]
      const cx = Math.floor(n.x / this.cell)
      const cy = Math.floor(n.y / this.cell)
      const k = cx * 73856093 + cy * 19349663
      const bucket = this.grid.get(k)
      if (bucket) bucket.push(i)
      else this.grid.set(k, [i])
    }
  }

  tick() {
    if (this.alpha < 0.005) return
    const { nodes, edges } = this
    this.rebuildGrid()

    // Repulsion is capped to the 3x3 cell neighbourhood - at a few thousand nodes the all-pairs version drops frames
    for (let i = 0; i < nodes.length; i++) {
      const a = nodes[i]
      const cx = Math.floor(a.x / this.cell)
      const cy = Math.floor(a.y / this.cell)
      for (let ox = -1; ox <= 1; ox++) {
        for (let oy = -1; oy <= 1; oy++) {
          const bucket = this.grid.get((cx + ox) * 73856093 + (cy + oy) * 19349663)
          if (!bucket) continue
          for (const j of bucket) {
            if (j <= i) continue
            const b = nodes[j]
            let dx = b.x - a.x
            let dy = b.y - a.y
            let d2 = dx * dx + dy * dy
            if (d2 > this.cell * this.cell * 4) continue
            if (d2 < 0.01) {
              dx = jitter(i + j)
              dy = jitter(i * 3 + j)
              d2 = dx * dx + dy * dy + 0.01
            }
            // Floor the denominator - an uncapped 1/d2 hands near-coincident nodes an impulse big enough to overflow to NaN within a few frames
            const force = (this.repulsion * this.alpha) / Math.max(d2, MIN_SEPARATION_SQ)
            const d = Math.sqrt(d2)
            const fx = (dx / d) * force
            const fy = (dy / d) * force
            a.vx -= fx
            a.vy -= fy
            b.vx += fx
            b.vy += fy
          }
        }
      }
    }

    for (const e of edges) {
      const a = nodes[e.source]
      const b = nodes[e.target]
      const dx = b.x - a.x
      const dy = b.y - a.y
      const d = Math.sqrt(dx * dx + dy * dy) || 0.01
      const pull = ((d - e.length) / d) * this.linkStrength * this.alpha
      const fx = dx * pull
      const fy = dy * pull
      a.vx += fx
      a.vy += fy
      b.vx -= fx
      b.vy -= fy
    }

    for (const n of nodes) {
      const target = this.radiusFor(n.ring)
      const d = Math.sqrt(n.x * n.x + n.y * n.y) || 0.01
      const pull = (target - d) * this.radialStrength * this.alpha
      n.vx += (n.x / d) * pull
      n.vy += (n.y / d) * pull

      // Links alone drag every node into one clump on one side of the ring - a weak pull back to the seeded
      // angle keeps each ring evenly spread while still letting neighbours sit together
      if (target > 0) {
        let delta = n.seedAngle - Math.atan2(n.y, n.x)
        delta = Math.atan2(Math.sin(delta), Math.cos(delta))
        const tangent = delta * target * this.angularStrength * this.alpha
        n.vx += (-n.y / d) * tangent
        n.vy += (n.x / d) * tangent
      }

      if (n.fixed) {
        n.x = 0
        n.y = 0
        n.vx = 0
        n.vy = 0
        continue
      }
      n.vx *= 0.82
      n.vy *= 0.82
      const speed = Math.sqrt(n.vx * n.vx + n.vy * n.vy)
      if (speed > MAX_SPEED) {
        n.vx = (n.vx / speed) * MAX_SPEED
        n.vy = (n.vy / speed) * MAX_SPEED
      }
      n.x += n.vx
      n.y += n.vy
      if (!Number.isFinite(n.x) || !Number.isFinite(n.y)) {
        n.x = Math.cos(n.ring) * target
        n.y = Math.sin(n.ring) * target
        n.vx = 0
        n.vy = 0
      }
    }

    this.alpha *= 0.985
  }
}
