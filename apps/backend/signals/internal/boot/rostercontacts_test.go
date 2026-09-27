package boot

import (
	"testing"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestADeactivatedContactReadsAsOptedOut(t *testing.T) {
	row := signalsdb.RosterContactRow{Email: "ada@test", MarketingOptedIn: true, SmsOptedIn: true}
	if c := contactFromRow(row); c.EmailOptedIn || c.SMSOptedIn {
		t.Errorf("inactive contact = email %v / sms %v, want both opted out", c.EmailOptedIn, c.SMSOptedIn)
	}
	row.IsActive = true
	if c := contactFromRow(row); !c.EmailOptedIn || !c.SMSOptedIn {
		t.Errorf("active contact = email %v / sms %v, want both opted in", c.EmailOptedIn, c.SMSOptedIn)
	}
}
