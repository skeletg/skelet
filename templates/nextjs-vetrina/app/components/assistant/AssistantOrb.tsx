"use client";

import { useEffect, useMemo, useRef } from "react";

// The assistant's mark, the same one the dashboard's assistant wears: a glowing core in petals of Skelet red that
// always breathe, and pulse deeper and quicker while it's answering. Drawn only while on screen

const SPREAD = 1.6;
const REST = 0.18;
const REST_FRAME_MS = 1000 / 24;

const PETALS = [
  { color: [255, 106, 61], lobes: 3, depth: 0.1, base: 0.66, turn: 0.15, pulse: 1.3, phase: 0 },
  { color: [234, 52, 57], lobes: 4, depth: 0.08, base: 0.7, turn: -0.11, pulse: 1.05, phase: 1.9 },
  { color: [255, 45, 111], lobes: 3, depth: 0.12, base: 0.63, turn: -0.19, pulse: 1.5, phase: 3.4 },
  { color: [255, 154, 107], lobes: 5, depth: 0.06, base: 0.72, turn: 0.08, pulse: 0.9, phase: 4.6 },
] as const;

type Spark = { angle: number; reach: number; size: number; speed: number; phase: number };
type Detail = { layers: number; sparks: number; rings: boolean };

function detailFor(px: number): Detail {
  if (px <= 24) return { layers: 3, sparks: 0, rings: false };
  if (px <= 40) return { layers: 4, sparks: 18, rings: true };
  return { layers: 4, sparks: 34, rings: true };
}

function sparks(n: number): Spark[] {
  let seed = 1013904223;
  const rand = () => {
    seed = (seed * 1664525 + 1013904223) >>> 0;
    return seed / 4294967296;
  };
  return Array.from({ length: n }, () => ({
    angle: rand() * 2 * Math.PI,
    reach: 0.95 + rand() * 0.6,
    size: 0.6 + rand() * 0.9,
    speed: 1 + rand() * 3,
    phase: rand() * 6,
  }));
}

const rgba = ([r, g, b]: readonly number[], a: number) => `rgba(${r}, ${g}, ${b}, ${a})`;

function draw(ctx: CanvasRenderingContext2D, px: number, detail: Detail, dust: Spark[], t: number, amp: number, dpr: number) {
  const side = ctx.canvas.width;
  const c = side / 2;
  const R = px / 2;
  ctx.clearRect(0, 0, side, side);

  if (detail.rings) {
    ctx.lineWidth = 0.6 * dpr;
    for (const [k, a] of [
      [1.12, 0.16],
      [1.42, 0.1],
    ] as const) {
      ctx.strokeStyle = rgba([234, 52, 57], a);
      ctx.beginPath();
      ctx.arc(c, c, R * k * (1 + 0.03 * amp * Math.sin(1.4 * t + k)), 0, 2 * Math.PI);
      ctx.stroke();
    }
  }

  for (const d of dust) {
    const a = d.angle + 0.05 * t;
    const glow = 0.25 + 0.6 * (0.5 + 0.5 * Math.sin(d.speed * t + d.phase));
    ctx.fillStyle = rgba([255, 90, 95], glow * (0.6 + 0.4 * amp));
    ctx.beginPath();
    ctx.arc(c + Math.cos(a) * R * d.reach, c + Math.sin(a) * R * d.reach, (d.size * dpr) / 2, 0, 2 * Math.PI);
    ctx.fill();
  }

  for (const p of PETALS.slice(0, detail.layers)) {
    const scale = 1 + (0.012 + 0.11 * amp) * Math.sin(p.pulse * (1 + amp) * t + p.phase);
    const turn = p.turn * t + p.phase;
    ctx.beginPath();
    for (let i = 0; i <= 72; i++) {
      const th = (i / 72) * 2 * Math.PI;
      const wobble = p.depth * (0.75 + 0.9 * amp) * Math.sin(p.lobes * th + turn) + 0.03 * Math.sin(2 * th - 0.7 * t + p.phase);
      const r = R * p.base * scale * (1 + wobble);
      if (i) ctx.lineTo(c + r * Math.cos(th), c + r * Math.sin(th));
      else ctx.moveTo(c + r * Math.cos(th), c + r * Math.sin(th));
    }
    ctx.closePath();
    ctx.fillStyle = rgba(p.color, 0.16);
    ctx.fill();
    ctx.strokeStyle = rgba(p.color, 0.85);
    ctx.lineWidth = Math.max(0.6, 0.8 * dpr);
    ctx.shadowColor = rgba(p.color, 0.6);
    ctx.shadowBlur = 4 * dpr;
    ctx.stroke();
    ctx.shadowBlur = 0;
  }

  const core = R * 0.46 * (1 + (0.012 + 0.07 * amp) * Math.sin(2.2 * t));
  const light = ctx.createRadialGradient(c, c, 0, c, c, core);
  light.addColorStop(0, "rgba(255, 255, 255, 1)");
  light.addColorStop(0.45, "rgba(255, 220, 222, 0.95)");
  light.addColorStop(0.8, "rgba(255, 110, 115, 0.85)");
  light.addColorStop(1, "rgba(234, 52, 57, 0.6)");
  ctx.fillStyle = light;
  ctx.shadowColor = "rgba(255, 80, 90, 0.8)";
  ctx.shadowBlur = 8 * dpr;
  ctx.beginPath();
  ctx.arc(c, c, core, 0, 2 * Math.PI);
  ctx.fill();
  ctx.shadowBlur = 0;
}

// rest is how much it moves while idle - a mark in a button barely stirs, the one greeting a new chat is alive
export function AssistantOrb({ size = 20, busy = false, rest = REST, className = "" }: { size?: number; busy?: boolean; rest?: number; className?: string }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const halo = useRef<HTMLSpanElement>(null);
  const amp = useRef(rest);
  const clock = useRef(0);
  const detail = useMemo(() => detailFor(size), [size]);
  const dust = useMemo(() => sparks(detail.sparks), [detail]);

  useEffect(() => {
    const el = canvas.current;
    const ctx = el?.getContext("2d");
    if (!el || !ctx) return;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const side = Math.round(size * SPREAD * dpr);
    if (el.width !== side) {
      el.width = side;
      el.height = side;
    }
    const paint = () => draw(ctx, size * dpr, detail, dust, clock.current, amp.current, dpr);
    if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) {
      paint();
      return;
    }
    let frame = 0;
    let visible = true;
    let last = performance.now();
    const tick = (now: number) => {
      frame = visible ? requestAnimationFrame(tick) : 0;
      if (!busy && Math.abs(amp.current - rest) < 0.01 && now - last < REST_FRAME_MS) return;
      const dt = Math.min(0.1, (now - last) / 1000);
      last = now;
      amp.current += ((busy ? 1 : rest) - amp.current) * Math.min(1, dt * (busy ? 5 : 2.5));
      clock.current += dt * (0.22 + 1.6 * amp.current);
      paint();
      if (halo.current) halo.current.style.opacity = String(0.1 + 0.3 * amp.current);
    };
    const seen =
      typeof IntersectionObserver === "function"
        ? new IntersectionObserver(([entry]) => {
            visible = entry.isIntersecting;
            if (visible && !frame) {
              last = performance.now();
              frame = requestAnimationFrame(tick);
            }
          })
        : null;
    seen?.observe(el);
    frame = requestAnimationFrame(tick);
    return () => {
      cancelAnimationFrame(frame);
      seen?.disconnect();
    };
  }, [busy, rest, size, detail, dust]);

  return (
    <span aria-hidden="true" className={`assistant-orb relative block shrink-0 rounded-full ${className}`} style={{ width: size, height: size }}>
      <span ref={halo} className="assistant-orb__halo" style={{ filter: `blur(${Math.max(8, size / 6)}px)` }} />
      <canvas ref={canvas} className="assistant-orb__voice" />
    </span>
  );
}
