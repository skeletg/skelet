package review

import (
	"testing"

	"github.com/google/uuid"
)

// submitting builds the event with its two mutable fields wired up, the way the service does before dispatch
func submitting() (*Submitting, *Status, *[]string) {
	status := StatusPending
	tags := []string{"seeded"}
	e := &Submitting{ProductID: uuid.New(), ChannelID: uuid.New(), Status: &status, Tags: &tags}
	return e, &status, &tags
}

func TestSubmittingWebhookEmptyBodyChangesNothing(t *testing.T) { // a webhook that answers 204 is saying "no opinion", and reading that as a rejection would silently drop every review
	e, status, tags := submitting()
	for name, body := range map[string][]byte{"nil": nil, "empty": {}} {
		reject, reason, err := e.ApplyWebhookResponse(body)
		if err != nil || reject || reason != "" {
			t.Errorf("%s body: reject=%v reason=%q err=%v, want a clean pass", name, reject, reason, err)
		}
	}
	if *status != StatusPending || len(*tags) != 1 {
		t.Errorf("status=%q tags=%v, want them untouched", *status, *tags)
	}
}

func TestSubmittingWebhookRejects(t *testing.T) {
	e, _, _ := submitting()
	reject, reason, err := e.ApplyWebhookResponse([]byte(`{"reject":true,"reason":"looks like spam"}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !reject {
		t.Error("reject = false, want the submission refused")
	}
	if reason != "looks like spam" {
		t.Errorf("reason = %q, want the webhook's reason surfaced to the submitter", reason)
	}
}

func TestSubmittingWebhookRejectionWinsOverItsOtherFields(t *testing.T) { // a body that both rejects and hands back an approved status is contradictory, and taking the status would publish content the guard just refused
	e, status, tags := submitting()
	reject, _, err := e.ApplyWebhookResponse([]byte(`{"reject":true,"status":"approved","tags":["profanity"]}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !reject {
		t.Fatal("reject = false, want the refusal honoured")
	}
	if *status != StatusPending {
		t.Errorf("status = %q on a rejected submission, want the proposed %q left alone", *status, StatusPending)
	}
	if len(*tags) != 1 {
		t.Errorf("tags = %v, want no tags applied to a submission that was refused outright", *tags)
	}
}

func TestSubmittingWebhookRewritesTheStatus(t *testing.T) { // this is the short-circuit a spam plugin uses - auto-reject or auto-approve without touching core moderation
	for _, want := range []Status{StatusApproved, StatusRejected, StatusPending} {
		e, status, _ := submitting()
		reject, _, err := e.ApplyWebhookResponse([]byte(`{"status":"` + string(want) + `"}`))
		if err != nil {
			t.Errorf("status %q: %v", want, err)
			continue
		}
		if reject {
			t.Errorf("status %q: reject = true, want the submission accepted", want)
		}
		if *status != want {
			t.Errorf("status = %q, want %q", *status, want)
		}
	}
}

func TestSubmittingWebhookRefusesAnUnknownStatus(t *testing.T) { // an unrecognized value written straight through would land a status the storefront filter has never heard of, so it errors instead
	for name, body := range map[string]string{
		"screaming": `{"status":"APPROVED"}`,
		"invented":  `{"status":"quarantined"}`,
		"numeric":   `{"status":"1"}`,
		"spaced":    `{"status":" approved"}`,
	} {
		e, status, _ := submitting()
		reject, _, err := e.ApplyWebhookResponse([]byte(body))
		if err == nil {
			t.Errorf("%s: err = nil, want the bad status refused", name)
		}
		if reject {
			t.Errorf("%s: reject = true, want the error reported rather than a silent rejection", name)
		}
		if *status != StatusPending {
			t.Errorf("%s: status = %q, want the proposed %q untouched", name, *status, StatusPending)
		}
	}
}

func TestSubmittingWebhookAppendsTagsRatherThanReplacing(t *testing.T) { // several webhooks run in turn and the AI classifier seeds its own, so replacing the slice would lose whoever ran first
	e, _, tags := submitting()
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"tags":["profanity"]}`)); err != nil {
		t.Fatalf("first webhook: %v", err)
	}
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"tags":["off-topic","spam"]}`)); err != nil {
		t.Fatalf("second webhook: %v", err)
	}
	want := []string{"seeded", "profanity", "off-topic", "spam"}
	if len(*tags) != len(want) {
		t.Fatalf("tags = %v, want %v", *tags, want)
	}
	for i := range want {
		if (*tags)[i] != want[i] {
			t.Fatalf("tags = %v, want %v", *tags, want)
		}
	}
}

func TestSubmittingWebhookLastStatusWins(t *testing.T) { // the documented resolution across several webhooks, worth pinning since the alternative (first wins) is just as plausible a reading
	e, status, _ := submitting()
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"status":"approved"}`)); err != nil {
		t.Fatalf("first webhook: %v", err)
	}
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"status":"rejected"}`)); err != nil {
		t.Fatalf("second webhook: %v", err)
	}
	if *status != StatusRejected {
		t.Errorf("status = %q, want the last webhook's %q", *status, StatusRejected)
	}
}

func TestSubmittingWebhookEmptyTagsAndStatusAreNoOps(t *testing.T) { // a webhook that answers with the full shape but no values must not blank the status or extend the tags
	e, status, tags := submitting()
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"reject":false,"reason":"","status":"","tags":[]}`)); err != nil {
		t.Fatalf("err = %v", err)
	}
	if *status != StatusPending {
		t.Errorf("status = %q, want the proposed %q", *status, StatusPending)
	}
	if len(*tags) != 1 {
		t.Errorf("tags = %v, want just the seeded one", *tags)
	}
}

func TestSubmittingWebhookWithNilTargets(t *testing.T) { // a caller that dispatches the event without the mutable fields wired must not panic on a webhook that fills them in
	e := &Submitting{ProductID: uuid.New(), ChannelID: uuid.New()}
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"status":"approved","tags":["spam"]}`)); err != nil {
		t.Errorf("err = %v, want the response absorbed", err)
	}
	reject, reason, err := e.ApplyWebhookResponse([]byte(`{"reject":true,"reason":"no"}`))
	if err != nil || !reject || reason != "no" {
		t.Errorf("reject=%v reason=%q err=%v, want the rejection still reported", reject, reason, err)
	}
}

func TestSubmittingWebhookMalformedBodyErrors(t *testing.T) { // a broken webhook must surface as a failure, not as an accidental approval
	e, status, _ := submitting()
	for name, body := range map[string]string{
		"truncated":            `{"reject":true`,
		"html":                 `<html>502 Bad Gateway</html>`,
		"an array":             `["reject"]`,
		"wrong type on reject": `{"reject":"yes"}`,
	} {
		if _, _, err := e.ApplyWebhookResponse([]byte(body)); err == nil {
			t.Errorf("%s: err = nil, want a decode failure", name)
		}
	}
	if *status != StatusPending {
		t.Errorf("status = %q, want it untouched by malformed bodies", *status)
	}
}

func TestSubmittingWebhookIgnoresUnknownFields(t *testing.T) { // a webhook returning its own envelope alongside the contract must not fail the submission
	e, status, _ := submitting()
	body := `{"status":"approved","score":0.91,"model":"spam-v2","meta":{"ms":12}}`
	if _, _, err := e.ApplyWebhookResponse([]byte(body)); err != nil {
		t.Fatalf("err = %v, want the extra fields ignored", err)
	}
	if *status != StatusApproved {
		t.Errorf("status = %q, want approved", *status)
	}
}

func TestReviewEventTypesAndChannels(t *testing.T) { // the bus routes on Type() and gates per-channel subscribers on EventChannelID(), so a wrong or empty value silently drops handlers
	ch := uuid.New()
	for want, ev := range map[string]interface {
		Type() string
		EventChannelID() uuid.UUID
	}{
		EventSubmitting: &Submitting{ChannelID: ch},
		EventSubmitted:  &Submitted{ChannelID: ch},
		EventApproved:   &Approved{ChannelID: ch},
		EventRejected:   &Rejected{ChannelID: ch},
	} {
		if got := ev.Type(); got != want {
			t.Errorf("Type() = %q, want %q", got, want)
		}
		if got := ev.EventChannelID(); got != ch {
			t.Errorf("%s: EventChannelID() = %s, want %s", want, got, ch)
		}
	}
}
