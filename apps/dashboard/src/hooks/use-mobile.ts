import * as React from "react"

const MOBILE_BREAKPOINT = 768

export function useIsMobile() {
  // Vite SPA - window exists at first render, so read lazily and let the effect only subscribe
  const [isMobile, setIsMobile] = React.useState<boolean>(
    () => window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`).matches,
  )

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`)
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    }
    mql.addEventListener("change", onChange)
    return () => mql.removeEventListener("change", onChange)
  }, [])

  return isMobile
}
