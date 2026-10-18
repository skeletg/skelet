package address

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Registry slug "remote" points at a merchant HTTP endpoint (remoteprovider kind=address_validator), the escape hatch for SmartyStreets/Loqate-style validation without a Go plugin - wire shapes are the request/response structs below
// fail_open, and no active provider at all, report valid so a down validator never breaks checkout; fail_closed returns ErrUnavailable instead
func RegisterRemoteValidator(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) {
	Register("remote", "Remote provider", &remoteValidator{providers: svc, invoker: invoker})
}

type providerLookup interface {
	ActiveFor(ctx context.Context, kind remoteprovider.Kind, channelID *uuid.UUID) (remoteprovider.Provider, bool, error)
}

type providerInvoker interface {
	Invoke(ctx context.Context, p remoteprovider.Provider, payload any, out any) error
}

type remoteValidator struct {
	providers providerLookup
	invoker   providerInvoker
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
	p, ok, err := r.providers.ActiveFor(ctx, remoteprovider.KindAddressValidator, &ch) // the channel's own provider beats a global one
	if err != nil {
		log.Printf("[address] remote validator lookup failed on channel %s (treating as valid): %v", channelID, err) // no provider row, so no policy to honour
		return Result{Valid: true}, nil
	}
	if !ok {
		return Result{Valid: true}, nil
	}

	payload := remoteAddressRequest{
		ChannelID: channelID.String(),
		Address:   remoteAddressInput(in), // same fields, differ only in JSON tags
	}
	var resp remoteAddressResponse
	if err := r.invoker.Invoke(ctx, p, payload, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return Result{}, fmt.Errorf("%w: provider %q: %w", ErrUnavailable, p.Slug, err)
		}
		log.Printf("[address] remote validator %q failed (treating as valid): %v", p.Slug, err)
		return Result{Valid: true}, nil
	}
	res := Result{Valid: resp.Valid, Messages: resp.Messages}
	if resp.Normalized != nil {
		norm := Input(*resp.Normalized)
		res.Normalized = &norm
	}
	return res, nil
}
