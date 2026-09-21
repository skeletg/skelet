package shipping

import "context"

const BuiltinCarrierSlug = "builtin"

// Adapts the built-in zones and methods engine to the Carrier interface so it quotes alongside plugin carriers - Quote delegates to Service.Quote, leaving built-in pricing byte-for-byte unchanged
// It does NOT implement Tracker or Labeler, since there's no carrier API, so those assertions fall through
type BuiltinCarrier struct {
	svc *Service
}

func NewBuiltinCarrier(svc *Service) *BuiltinCarrier {
	return &BuiltinCarrier{svc: svc}
}

// Maps the carrier RateRequest onto the engine's QuoteInput and back - method codes carry through, so placement can re-resolve code to method by destination through Service.ComputeForOrder
func (c *BuiltinCarrier) Quote(ctx context.Context, req RateRequest) ([]RatedMethod, error) {
	quotes, err := c.svc.Quote(ctx, QuoteInput{
		ChannelID:     req.ChannelID,
		CountryCode:   req.Destination.CountryCode,
		SubtotalCents: req.SubtotalCents,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RatedMethod, len(quotes))
	for i, q := range quotes {
		out[i] = RatedMethod{
			Code:       q.Code,
			Name:       q.Name,
			PriceCents: q.PriceCents,
		}
	}
	return out, nil
}
