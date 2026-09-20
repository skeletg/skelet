package auth

import "testing"

func TestHashToken(t *testing.T) {
	h1, h2 := HashToken("abc"), HashToken("abc")
	if h1 != h2 {
		t.Error("HashToken not deterministic for identical input")
	}
	if HashToken("abc") == HashToken("abd") {
		t.Error("HashToken collided on distinct inputs")
	}
	if HashToken("abc") != hashToken("abc") {
		t.Error("HashToken and hashToken disagree")
	}
	// Known SHA-256 vector for the empty string
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := HashToken(""); got != emptySHA256 {
		t.Errorf("HashToken(\"\"): got %s, want %s", got, emptySHA256)
	}
	if got := HashToken("anything"); len(got) != 64 {
		t.Errorf("HashToken length: got %d, want 64", len(got))
	}
}
