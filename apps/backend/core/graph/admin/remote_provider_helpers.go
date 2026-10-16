package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// A global provider answers checkout on every channel, so it needs the global grant
func (r *Resolver) requireRemoteProviderScope(ctx context.Context, id uuid.UUID, slug string) (remoteprovider.Provider, error) {
	p, err := r.RemoteProviderSvc.GetByID(ctx, id)
	if err != nil {
		return remoteprovider.Provider{}, err
	}
	return p, auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, slug, p.ChannelID)
}

func toRemoteProviderModel(p remoteprovider.Provider) *model.RemoteProvider {
	m := &model.RemoteProvider{
		ID:            p.ID.String(),
		Kind:          remoteProviderKindToGQL(p.Kind),
		Slug:          p.Slug,
		Name:          p.Name,
		TargetURL:     p.TargetURL,
		TimeoutMs:     p.TimeoutMs,
		CacheTTLS:     p.CacheTTLs,
		FailurePolicy: remoteProviderPolicyToGQL(p.FailurePolicy),
		IsActive:      p.IsActive,
		Circuit:       toRemoteProviderCircuitModel(p.ID),
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
	if len(p.Config) > 0 {
		var config any
		if err := json.Unmarshal(p.Config, &config); err == nil {
			m.Config = config
		}
	}
	// thin channel shell (id only) - same N+1-avoidance as toWebhookModel
	if p.ChannelID != nil {
		m.Channel = &model.Channel{ID: p.ChannelID.String()}
	}
	return m
}

// config is where merchants keep the receiver's API keys - a view-only admin sees the provider, not its credentials
func redactedUnlessManaged(m *model.RemoteProvider, manages bool) *model.RemoteProvider {
	if !manages {
		m.Config = nil
	}
	return m
}

func toRemoteProviderCircuitModel(id uuid.UUID) *model.WebhookCircuit {
	return toCircuitModel(remoteprovider.CircuitInfoFor(id))
}

func remoteProviderKindToGQL(k remoteprovider.Kind) model.RemoteProviderKind {
	switch k {
	case remoteprovider.KindShippingCarrier:
		return model.RemoteProviderKindShippingCarrier
	case remoteprovider.KindTaxProvider:
		return model.RemoteProviderKindTaxProvider
	case remoteprovider.KindAddressValidator:
		return model.RemoteProviderKindAddressValidator
	case remoteprovider.KindPromotionCondition:
		return model.RemoteProviderKindPromotionCondition
	case remoteprovider.KindPromotionAction:
		return model.RemoteProviderKindPromotionAction
	case remoteprovider.KindFraudRisk:
		return model.RemoteProviderKindFraudRisk
	case remoteprovider.KindCurrencyRate:
		return model.RemoteProviderKindCurrencyRate
	case remoteprovider.KindPricing:
		return model.RemoteProviderKindPricing
	case remoteprovider.KindRecommendation:
		return model.RemoteProviderKindRecommendation
	case remoteprovider.KindReviews:
		return model.RemoteProviderKindReviews
	case remoteprovider.KindSearch:
		return model.RemoteProviderKindSearch
	case remoteprovider.KindCustomerSegments:
		return model.RemoteProviderKindCustomerSegments
	case remoteprovider.KindReturnsPolicy:
		return model.RemoteProviderKindReturnsPolicy
	case remoteprovider.KindInventoryAvail:
		return model.RemoteProviderKindInventoryAvailability
	case remoteprovider.KindShippingTracking:
		return model.RemoteProviderKindShippingTracking
	case remoteprovider.KindDeliveryPoints:
		return model.RemoteProviderKindDeliveryPoints
	default:
		return model.RemoteProviderKindShippingCarrier
	}
}

func remoteProviderKindFromGQL(k model.RemoteProviderKind) remoteprovider.Kind {
	switch k {
	case model.RemoteProviderKindShippingCarrier:
		return remoteprovider.KindShippingCarrier
	case model.RemoteProviderKindTaxProvider:
		return remoteprovider.KindTaxProvider
	case model.RemoteProviderKindAddressValidator:
		return remoteprovider.KindAddressValidator
	case model.RemoteProviderKindPromotionCondition:
		return remoteprovider.KindPromotionCondition
	case model.RemoteProviderKindPromotionAction:
		return remoteprovider.KindPromotionAction
	case model.RemoteProviderKindFraudRisk:
		return remoteprovider.KindFraudRisk
	case model.RemoteProviderKindCurrencyRate:
		return remoteprovider.KindCurrencyRate
	case model.RemoteProviderKindPricing:
		return remoteprovider.KindPricing
	case model.RemoteProviderKindRecommendation:
		return remoteprovider.KindRecommendation
	case model.RemoteProviderKindReviews:
		return remoteprovider.KindReviews
	case model.RemoteProviderKindSearch:
		return remoteprovider.KindSearch
	case model.RemoteProviderKindCustomerSegments:
		return remoteprovider.KindCustomerSegments
	case model.RemoteProviderKindReturnsPolicy:
		return remoteprovider.KindReturnsPolicy
	case model.RemoteProviderKindInventoryAvailability:
		return remoteprovider.KindInventoryAvail
	case model.RemoteProviderKindShippingTracking:
		return remoteprovider.KindShippingTracking
	case model.RemoteProviderKindDeliveryPoints:
		return remoteprovider.KindDeliveryPoints
	default:
		return remoteprovider.KindShippingCarrier
	}
}

func remoteProviderPolicyToGQL(p remoteprovider.FailurePolicy) model.RemoteProviderFailurePolicy {
	if p == remoteprovider.FailClosed {
		return model.RemoteProviderFailurePolicyFailClosed
	}
	return model.RemoteProviderFailurePolicyFailOpen
}

func remoteProviderPolicyFromGQL(p model.RemoteProviderFailurePolicy) remoteprovider.FailurePolicy {
	if p == model.RemoteProviderFailurePolicyFailClosed {
		return remoteprovider.FailClosed
	}
	return remoteprovider.FailOpen
}

// remoteProviderConfigFromGQL re-marshals the JSON scalar so the stored
// config is exactly what the admin sent, validated as JSON.
func remoteProviderConfigFromGQL(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("config must be JSON-serializable: %w", err)
	}
	return raw, nil
}
