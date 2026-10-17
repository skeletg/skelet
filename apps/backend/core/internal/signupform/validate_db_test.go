package signupform

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
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

// Confirming records the form's consent, so a kind the shopper can't decline, or one that doesn't exist, would fail every confirmation
func TestAFormAsksOnlyForAMarketingConsent(t *testing.T) {
	f := newConfirmFixture(t)
	for _, kind := range []string{consentkind.TransactionalEmail, "newsletter_v2"} {
		if _, err := f.svc.Create(f.ctx, SaveInput{ChannelID: f.form.ChannelID, Slug: "k-" + uuid.NewString()[:6], Name: "K", ConsentKind: kind}); !errors.Is(err, ErrConsentKindInvalid) {
			t.Errorf("Create(%q) = %v, want ErrConsentKindInvalid", kind, err)
		}
		if _, err := f.svc.Update(f.ctx, f.form.ID, SaveInput{Name: "Newsletter", ConsentKind: kind}); !errors.Is(err, ErrConsentKindInvalid) {
			t.Errorf("Update(%q) = %v, want ErrConsentKindInvalid", kind, err)
		}
	}
	got, err := f.svc.Update(f.ctx, f.form.ID, SaveInput{Name: "Newsletter", Status: "active", ConsentKind: consentkind.MarketingSMS})
	if err != nil || got.ConsentKind != consentkind.MarketingSMS {
		t.Fatalf("Update(marketing_sms) = %+v, %v", got.ConsentKind, err)
	}
}

func TestASubmitFindsOnlyThisChannelsActiveForm(t *testing.T) {
	f := newConfirmFixture(t)
	if _, err := f.svc.Submit(f.ctx, uuid.New(), f.form.Slug, f.email, ""); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("another channel's slug: %v, want ErrFormNotFound", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.ID.String(), f.email, ""); err != nil {
		t.Errorf("by id: %v", err)
	}
	if _, err := f.svc.Update(f.ctx, f.form.ID, SaveInput{Name: "Newsletter", Status: "draft"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, f.email, ""); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("a draft form: %v, want ErrFormNotFound", err)
	}
}

func TestRecentSubmissionsCapsTheLimitAt500(t *testing.T) {
	f := newConfirmFixture(t)
	if _, err := f.conn.Exec(`INSERT INTO signup_form_submissions (form_id, channel_id, email)
		SELECT $1, $2, 'sub' || g || '@form.test' FROM generate_series(1, 501) g`, f.form.ID, f.form.ChannelID); err != nil {
		t.Fatal(err)
	}
	for limit, want := range map[int32]int{1000: 500, 0: 100, 3: 3} {
		if rows, err := f.svc.RecentSubmissions(f.ctx, f.form.ID, limit); err != nil || len(rows) != want {
			t.Errorf("RecentSubmissions(%d) = %d rows, %v - want %d", limit, len(rows), err, want)
		}
	}
}
