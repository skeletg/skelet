package campaign

import "testing"

func TestValidAudience(t *testing.T) {
	for _, k := range []string{AudienceAll, AudienceGroup, AudienceSegment} {
		if !validAudience(k) {
			t.Errorf("validAudience(%q) = false", k)
		}
	}
	if validAudience("bogus") {
		t.Error("validAudience accepted an unknown kind")
	}
}
