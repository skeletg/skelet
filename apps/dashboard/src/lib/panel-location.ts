import { useContext, useMemo } from 'react'
import { NavigationType, UNSAFE_LocationContext, type Location } from 'react-router-dom'

// The location a workspace tab's page sees - the router's while the tab is live, its own frozen path otherwise. While
// a tab-switch navigation is still in flight the router reports the OLD page, so the newly live panel keeps its frozen
// location for that beat and the wrong page can't flash through it. A background panel's value must not depend on the
// router context at all: that object changes on every navigation, and every location consumer in every hidden tab
// would re-render with it
export function usePanelLocation(live: boolean, lagging: boolean, frozen: Location) {
  const parent = useContext(UNSAFE_LocationContext)
  const showLive = live && !lagging
  const location = showLive ? parent.location : frozen
  const navigationType = showLive ? parent.navigationType : NavigationType.Pop
  return useMemo(() => ({ location, navigationType }), [location, navigationType])
}
