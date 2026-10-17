package emailtrack

import (
	"testing"

	"github.com/google/uuid"
)

// Unsubscribe links live in inboxes for months, so a rotated key keeps the old ones working while it's listed as previous
func TestARotatedLinkKeyKeepsOldLinksWorking(t *testing.T) {
	id := uuid.New()
	oldLink := NewSigner([]byte("old-link-key")).SignUnsubscribe(id)
	oldPixel := NewSigner([]byte("old-link-key")).Sign(Payload{S: id})
	rotated := NewSigner([]byte("new-link-key"), []byte("old-link-key"))
	if got, err := rotated.VerifyUnsubscribe(oldLink); err != nil || got != id {
		t.Fatalf("an unsubscribe link from the previous key = %v, %v", got, err)
	}
	if p, err := rotated.Verify(oldPixel); err != nil || p.S != id {
		t.Fatalf("a tracking token from the previous key = %v, %v", p, err)
	}
	if _, err := NewSigner([]byte("new-link-key")).VerifyUnsubscribe(oldLink); err == nil {
		t.Fatal("a key no longer listed still verified")
	}
	if fresh := rotated.SignUnsubscribe(id); fresh == oldLink {
		t.Fatal("the rotated signer still signs with the old key")
	}
}
