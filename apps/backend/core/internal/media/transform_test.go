package media

import (
	"testing"
)

func TestSnapWidth(t *testing.T) {
	cases := map[int]int{-5: 160, 0: 160, 1: 160, 160: 160, 161: 320, 900: 960, 2048: 2048, 99999: 2048}
	for in, want := range cases {
		if got := SnapWidth(in); got != want {
			t.Errorf("SnapWidth(%d) = %d, want %d", in, got, want)
		}
	}
}

// The geometry both render paths share - a build switch must not move a crop
func TestCoverBox(t *testing.T) {
	// Downscale cover: 64x32 source into a 32x32 box covers on height
	if scale, w, h := coverBox(64, 32, 32, 32); scale != 1.0 || w != 32 || h != 32 {
		t.Errorf("coverBox(64,32,32,32) = %v,%d,%d want 1,32,32", scale, w, h)
	}
	// Never upscale - a 32x32 box on a 16x16 source shrinks the box instead
	if scale, w, h := coverBox(16, 16, 32, 32); scale != 1.0 || w != 16 || h != 16 {
		t.Errorf("coverBox(16,16,32,32) = %v,%d,%d want 1,16,16", scale, w, h)
	}
}

func TestCropOrigin(t *testing.T) {
	// Left focal pins the window to the left edge, right focal to the right
	if x, y := cropOrigin(64, 32, 32, 32, 0.1, 0.5); x != 0 || y != 0 {
		t.Errorf("left focal = %d,%d want 0,0", x, y)
	}
	if x, _ := cropOrigin(64, 32, 32, 32, 0.9, 0.5); x != 32 {
		t.Errorf("right focal x = %d want 32", x)
	}
	// 0,0 means centre
	if x, _ := cropOrigin(64, 32, 32, 32, 0, 0); x != 16 {
		t.Errorf("centre focal x = %d want 16", x)
	}
	// Out-of-range focal clamps instead of escaping the image
	if x, y := cropOrigin(64, 32, 32, 32, 5, -3); x != 32 || y != 0 {
		t.Errorf("clamped focal = %d,%d want 32,0", x, y)
	}
}
