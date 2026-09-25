package campaign

import (
	"context"

	"github.com/google/uuid"
)

// What a broadcast needs from outside its own tables, answered from the rosters core pushes across
// The types are campaign's own: naming the whole customer record would tie the send loop to a commerce model it cannot read

// One addressable person, with the marketing consent already resolved by whoever holds it
type Contact struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Email     string
	FirstName string
	// uuid.Nil when the customer is in no group, which no group audience matches
	GroupID uuid.UUID
	Active  bool
	OptedIn bool
}

// The audience a send is drawn from
type audienceReader interface {
	// Everyone the channel could mail, consent included - the caller filters down to the audience kind
	ListForChannel(ctx context.Context, channelID uuid.UUID) ([]Contact, error)
	// One person, for the paths that already know who they are
	Get(ctx context.Context, customerID uuid.UUID) (Contact, bool, error)
	// Consent as it stands right now - the send loop works from stored recipient rows, and someone
	// who unsubscribed after the campaign was built must not be mailed
	OptedInByIDs(ctx context.Context, customerIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// The shop a send goes out as - the customer-facing name, the storefront the links point at, and
// the sender identity core resolved from its settings, all off the channel roster
type Store struct {
	ID          uuid.UUID
	Name        string
	PublicURL   string
	FromAddress string
	FromName    string
	LogoURL     string
	Active      bool
}

type storeReader interface {
	Get(ctx context.Context, channelID uuid.UUID) (Store, error)
}
