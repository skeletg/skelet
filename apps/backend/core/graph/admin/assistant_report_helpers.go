package admin

import "math"

func count32(n int64) int32 { return int32(min(n, math.MaxInt32)) }
