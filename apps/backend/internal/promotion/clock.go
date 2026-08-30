package promotion

import "time"

var nowFunc = time.Now // test seam for the engine's validity-window checks

func NowFunc() time.Time { return nowFunc() } // conditions read the request-time clock here; tests patch nowFunc, so engine-level overrides reach them too

// Pins the clock and returns the restore func - for condition tests in other packages, which can't reach nowFunc directly
// Never call from production paths
func PinNowForTest(at time.Time) func() {
	prev := nowFunc
	nowFunc = func() time.Time { return at }
	return func() { nowFunc = prev }
}
