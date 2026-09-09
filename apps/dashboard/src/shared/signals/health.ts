// Signals runs as its own service, so when it is down every query fails at the network and its pages render as though there were simply no data - tracked once here, since one unreachable service is one fact rather than a fact per screen

type Listener = (down: boolean) => void

let down = false
const listeners = new Set<Listener>()

export function signalsIsDown(): boolean {
	return down
}

export function setSignalsDown(next: boolean) {
	if (down === next) return
	down = next
	for (const l of listeners) l(down)
}

export function subscribeSignalsHealth(fn: Listener): () => void {
	listeners.add(fn)
	return () => {
		listeners.delete(fn)
	}
}
