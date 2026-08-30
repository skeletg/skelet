package address

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

// Registry slug "remote" points at a merchant HTTP endpoint (remoteprovider kind=address_validator), the escape hatch for SmartyStreets/Loqate-style validation without a Go plugin - wire shapes are the request/response structs below
// fail_open, and no active provider at all, report valid so a down validator never breaks checkout; fail_closed surfaces the error instead
func RegisterRemoteValidator(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) {
	Register("remote", &remoteValidator{svc: svc, invoker: invoker})
}

type remoteValidator struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

type remoteAddressRequest struct {
	ChannelID string             `json:"channelId"`
	Address   remoteAddressInput `json:"address"`
}

type remoteAddressInput struct {
	Street1     string `json:"street1"`
	Street2     string `json:"street2,omitempty"`
	City        string `json:"city"`
	State       string `json:"state,omitempty"`
	PostalCode  string `json:"postalCode"`
	CountryCode string `json:"countryCode"`
}

type remoteAddressResponse struct {
	Valid      bool                `json:"valid"`
	Messages   []string            `json:"messages"`
	Normalized *remoteAddressInput `json:"normalized"`
}

func (r *remoteValidator) Validate(ctx context.Context, channelID uuid.UUID, in Input) (Result, error) {
	ch := channelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindAddressValidator, &ch)
	if err != nil || len(providers) == 0 {
		return Result{Valid: true}, nil // no provider - treat as valid
	}
	p := providers[0]

	payload := remoteAddressRequest{
		ChannelID: channelID.String(),
		Address:   remoteAddressInput(in), // same fields, differ only in JSON tags
	}
	var resp remoteAddressResponse
	if err := r.invoker.Invoke(ctx, p, payload, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return Result{}, err
		}
		log.Printf("[address] remote validator %q failed (treating as valid): %v", p.Slug, err) // fail_open - a down validator never blocks the shopper
		return Result{Valid: true}, nil
	}
	res := Result{Valid: resp.Valid, Messages: resp.Messages}
	if resp.Normalized != nil {
		res.Normalized = &Input{
			Street1: resp.Normalized.Street1, Street2: resp.Normalized.Street2,
			City: resp.Normalized.City, State: resp.Normalized.State,
			PostalCode: resp.Normalized.PostalCode, CountryCode: resp.Normalized.CountryCode,
		}
	}
	return res, nil
}
