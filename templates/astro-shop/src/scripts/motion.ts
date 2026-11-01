// Scroll reveals and the cart badge pop - Motion's mini animate() runs on the browser's own
// Web Animations API, so this whole file ships at a few KB and never touches the main thread per frame
import { inView } from "motion";
import { animate } from "motion/mini";

const EASE = [0.22, 1, 0.36, 1] as const;

const wired = new WeakSet<Document>();

export function initMotion(doc: Document = document, win: Window = window): void {
  popBadge(doc, win);
  wireVideoToggles(doc);
  if (win.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

  // Only what starts below the fold is hidden, so on-screen content and the LCP image paint untouched
  const below = [...doc.querySelectorAll<HTMLElement>("[data-reveal]")].filter((el) => el.getBoundingClientRect().top > win.innerHeight);
  for (const el of below) {
    el.style.opacity = "0";
    el.style.transform = "translateY(18px)";
  }
  // Cards entering together stagger; the counter resets once a row has landed
  let batch = 0;
  let reset: ReturnType<typeof setTimeout> | undefined;
  if (below.length > 0) {
    inView(
      below,
      (el) => {
        const i = batch++;
        clearTimeout(reset);
        reset = setTimeout(() => (batch = 0), 120);
        const target = el as HTMLElement;
        animate(target, { opacity: [0, 1], transform: ["translateY(18px)", "translateY(0px)"] }, { duration: 0.55, delay: i * 0.06, ease: EASE }).then(() => {
          target.style.removeProperty("opacity");
          target.style.removeProperty("transform");
        });
      },
      { margin: "0px 0px -8% 0px" },
    );
  }
}

function wireVideoToggles(doc: Document): void {
  if (wired.has(doc)) return;
  wired.add(doc);
  doc.addEventListener("click", (e) => {
    const btn = (e.target as Element).closest<HTMLButtonElement>("button[data-video-toggle]");
    const video = btn ? doc.getElementById(btn.dataset.videoToggle ?? "") : null;
    if (!btn || !(video instanceof HTMLVideoElement)) return;
    if (video.paused) void video.play().catch(() => {});
    else video.pause();
    btn.setAttribute("aria-pressed", String(video.paused));
    btn.textContent = video.paused ? "Play video" : "Pause video";
  });
}

// Set by the cart when a line was just added - pop the badge once, then forget it. Also called after an add
// that posted in the background (enhance.ts), where no page load follows
export function popBadge(doc: Document = document, win: Window = window): void {
  if (!/(?:^|; )cart_bump=1/.test(doc.cookie)) return;
  doc.cookie = "cart_bump=; Max-Age=0; Path=/";
  if (win.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  const badge = doc.querySelector<HTMLElement>("[data-cart-count]");
  if (badge && !badge.hidden) animate(badge, { transform: ["scale(1)", "scale(1.35)", "scale(1)"] }, { duration: 0.45, ease: EASE });
}
