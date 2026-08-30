package backinstock

import "github.com/google/uuid"

const EventRequested = "backinstock.requested" // fires when a shopper subscribes to a restock alert

type Requested struct {
	ChannelID  uuid.UUID
	ProductID  uuid.UUID
	SKU        string
	Email      string
	CustomerID *uuid.UUID
}

func (*Requested) Type() string                { return EventRequested }
func (e *Requested) EventChannelID() uuid.UUID { return e.ChannelID }
