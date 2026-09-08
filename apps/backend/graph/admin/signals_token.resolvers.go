package admin

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/signalsapi"
)

func (r *queryResolver) SignalsToken(ctx context.Context) (*model.ServiceToken, error) {
	// No permission check beyond being signed in - the token carries the caller's own grants, so it opens nothing they lack
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	sc := auth.ServiceClaims{
		AdminID:     claims.AdminID,
		SessionID:   claims.SessionID,
		Email:       claims.Email,
		IsSuperuser: claims.IsSuperuser,
	}
	// A superuser bypasses every check before a grant is read, so their token stays empty rather than carrying all 86
	if !claims.IsSuperuser {
		grants, err := r.AdminSvc.Grants(ctx, claims.AdminID)
		if err != nil {
			return nil, err
		}
		sc.Grants = auth.FilterGrants(grants, signalsapi.Slugs)
	}
	// Same lifetime as the admin access token it was exchanged for, so both refresh on one cadence
	expires := time.Now().Add(r.JWTTTL)
	token, err := auth.GenerateServiceToken(sc, signalsapi.Audience, r.JWTSecret, r.JWTTTL)
	if err != nil {
		return nil, err
	}
	return &model.ServiceToken{Token: token, ExpiresAt: expires}, nil
}
