package signupform

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The address is mailed at once, so anything that isn't exactly one address is refused before a confirmation goes out
func TestSubmitMailsOnlyAPlainAddress(t *testing.T) {
	f := newConfirmFixture(t)
	for _, bad := range []string{"not-an-address", "a@", "ana@example.test, bob@example.test", "Ana <ana@example.test>", "ana@example.test\r\nBcc: x@example.test", strings.Repeat("a", 250) + "@b.cd"} {
		if _, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, bad, ""); !errors.Is(err, ErrEmailInvalid) {
			t.Errorf("Submit(%q) = %v, want ErrEmailInvalid", bad, err)
		}
	}
	if _, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, f.email, strings.Repeat("ž", maxNameRunes+1)); !errors.Is(err, ErrNameTooLong) {
		t.Errorf("an oversized name: %v, want ErrNameTooLong", err)
	}
	if len(f.mail.to) != 0 {
		t.Fatalf("%d confirmations mailed for refused submissions", len(f.mail.to))
	}
	if _, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, "  "+strings.ToUpper(f.email)+" ", strings.Repeat("ž", maxNameRunes)); err != nil {
		t.Fatalf("a plain address refused: %v", err)
	}
	if len(f.mail.to) != 1 || f.mail.to[0] != f.email {
		t.Fatalf("mailed %v, want the normalized address", f.mail.to)
	}
}

func TestFormErrorsAreTyped(t *testing.T) {
	f := newConfirmFixture(t)
	if _, err := f.svc.Create(f.ctx, SaveInput{ChannelID: f.form.ChannelID, Slug: f.form.Slug, Name: "Again"}); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("a duplicate slug: %v, want ErrSlugTaken", err)
	}
	if _, err := f.svc.Update(f.ctx, f.form.ID, SaveInput{Name: "  "}); !errors.Is(err, ErrNameAndSlugRequired) {
		t.Errorf("blanking the name: %v, want ErrNameAndSlugRequired", err)
	}
	if _, err := f.svc.Get(f.ctx, uuid.New()); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("Get of a missing form: %v, want ErrFormNotFound", err)
	}
	if _, err := f.svc.Update(f.ctx, uuid.New(), SaveInput{Name: "x"}); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("Update of a missing form: %v, want ErrFormNotFound", err)
	}
}

func TestDisplaySettingsAreHeldToWhatTheWidgetCanDo(t *testing.T) {
	f := newConfirmFixture(t)
	got, err := f.svc.Update(f.ctx, f.form.ID, SaveInput{Name: "Newsletter", Display: DisplayConfig{Trigger: "on_full_moon", DelaySeconds: -5, ScrollPercent: 300, FrequencyDays: 9999}})
	if err != nil {
		t.Fatal(err)
	}
	if want := (DisplayConfig{Trigger: "immediate", DelaySeconds: 0, ScrollPercent: 100, FrequencyDays: 365}); got.Display != want {
		t.Fatalf("stored %+v, want %+v", got.Display, want)
	}
}

func TestTheMailedLifetimeIsTheTTL(t *testing.T) {
	if confirmationTTLText != "7 days" {
		t.Fatalf("confirmation mail says %q for a %s TTL", confirmationTTLText, ConfirmationTTL)
	}
}
