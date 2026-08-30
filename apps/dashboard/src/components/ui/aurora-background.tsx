import { cn } from "@/lib/utils";

// Low-power ambient backdrop for the auth and error pages - a few soft radial gradients drift
// using ONLY CSS transform/opacity keyframes, so the compositor animates them on the GPU with
// no per-frame layout, no rAF and no JS, unlike the animated-SVG beams it replaced
// Honors prefers-reduced-motion by freezing to a static gradient
export function AuroraBackground({ className }: { className?: string }) {
  return (
    <div
      aria-hidden
      className={cn(
        "pointer-events-none absolute inset-0 overflow-hidden",
        className,
      )}
    >
      <div className="skelet-aurora skelet-aurora--a" />
      <div className="skelet-aurora skelet-aurora--b" />
      <div className="skelet-aurora skelet-aurora--c" />
      <style>{`
        .skelet-aurora {
          position: absolute;
          border-radius: 50%;
          will-change: transform;
        }
        .skelet-aurora--a {
          width: 60vw; height: 60vw; left: -15vw; top: -20vw;
          background: radial-gradient(circle, rgba(24,204,252,0.40), rgba(24,204,252,0) 70%);
          animation: skelet-aurora-a 24s ease-in-out infinite alternate;
        }
        .skelet-aurora--b {
          width: 55vw; height: 55vw; right: -15vw; top: 8vh;
          background: radial-gradient(circle, rgba(174,72,255,0.35), rgba(174,72,255,0) 70%);
          animation: skelet-aurora-b 30s ease-in-out infinite alternate;
        }
        .skelet-aurora--c {
          width: 50vw; height: 50vw; left: 18vw; bottom: -22vw;
          background: radial-gradient(circle, rgba(99,68,245,0.35), rgba(99,68,245,0) 70%);
          animation: skelet-aurora-c 27s ease-in-out infinite alternate;
        }
        @keyframes skelet-aurora-a {
          from { transform: translate3d(0, 0, 0) scale(1); }
          to   { transform: translate3d(10vw, 6vh, 0) scale(1.15); }
        }
        @keyframes skelet-aurora-b {
          from { transform: translate3d(0, 0, 0) scale(1.05); }
          to   { transform: translate3d(-8vw, 8vh, 0) scale(0.92); }
        }
        @keyframes skelet-aurora-c {
          from { transform: translate3d(0, 0, 0) scale(1); }
          to   { transform: translate3d(6vw, -8vh, 0) scale(1.12); }
        }
        @media (prefers-reduced-motion: reduce) {
          .skelet-aurora { animation: none !important; }
        }
      `}</style>
    </div>
  );
}
