// Shared "a navigation is in flight" signal
//
// Next has no global hook for this - useLinkStatus only reports for the Link it sits under, so
// every LocaleLink renders a probe that reports in here, and router.push() reports its transition

let inFlight = 0;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

export function beginNavigation() {
  inFlight++;
  emit();
}

export function endNavigation() {
  // Clamped - a probe unmounting mid-navigation would otherwise drive this negative and wedge
  // the bar on for the rest of the session
  inFlight = Math.max(0, inFlight - 1);
  emit();
}

export function subscribeNavigation(onChange: () => void) {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

export function navigationPending(): boolean {
  return inFlight > 0;
}

// The server render has nothing in flight, and saying otherwise would hydrate a visible bar
export function navigationPendingServer(): boolean {
  return false;
}
