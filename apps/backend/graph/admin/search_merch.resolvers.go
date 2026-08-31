package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
)

// AddSearchSynonym records a comma-separated synonym equivalence group.
func (r *mutationResolver) AddSearchSynonym(ctx context.Context, channelID string, terms string) (*model.SearchSynonym, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	syn, err := r.SearchMerchSvc.AddSynonym(ctx, &cid, terms)
	if err != nil {
		return nil, err
	}
	return &model.SearchSynonym{ID: syn.ID.String(), Terms: syn.Terms}, nil
}

// DeleteSearchSynonym removes a synonym group.
func (r *mutationResolver) DeleteSearchSynonym(ctx context.Context, channelID string, id string) (bool, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	sid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", nil)
	}
	if err := r.SearchMerchSvc.DeleteSynonym(ctx, cid, sid); err != nil {
		return false, err
	}
	return true, nil
}

// UpsertSearchPin pins a product to a query's results.
func (r *mutationResolver) UpsertSearchPin(ctx context.Context, channelID string, query string, productID string, position *int32) (*model.SearchPin, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return nil, errInvalidID("product id", nil)
	}
	pos := int32(1)
	if position != nil {
		pos = *position
	}
	pin, err := r.SearchMerchSvc.UpsertPin(ctx, cid, query, pid, pos)
	if err != nil {
		return nil, err
	}
	return &model.SearchPin{ID: pin.ID.String(), Query: pin.Query, ProductID: pin.ProductID.String(), Position: pin.Position}, nil
}

// DeleteSearchPin removes a pin.
func (r *mutationResolver) DeleteSearchPin(ctx context.Context, channelID string, id string) (bool, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	pid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", nil)
	}
	if err := r.SearchMerchSvc.DeletePin(ctx, cid, pid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertSearchBoost(ctx context.Context, channelID string, query string, productID string, weight float64) (*model.SearchBoost, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return nil, errInvalidID("product id", nil)
	}
	b, err := r.SearchMerchSvc.UpsertBoost(ctx, cid, query, pid, weight)
	if err != nil {
		return nil, err
	}
	return &model.SearchBoost{ID: b.ID.String(), Query: b.Query, ProductID: b.ProductID.String(), Weight: b.Weight}, nil
}

func (r *mutationResolver) DeleteSearchBoost(ctx context.Context, channelID string, id string) (bool, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	bid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("boost id", nil)
	}
	if err := r.SearchMerchSvc.DeleteBoost(ctx, cid, bid); err != nil {
		return false, err
	}
	return true, nil
}

// UpsertSearchRedirect maps a query to a URL.
func (r *mutationResolver) UpsertSearchRedirect(ctx context.Context, channelID string, query string, targetURL string) (*model.SearchRedirect, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	red, err := r.SearchMerchSvc.UpsertRedirect(ctx, cid, query, targetURL)
	if err != nil {
		return nil, err
	}
	return &model.SearchRedirect{ID: red.ID.String(), Query: red.Query, TargetURL: red.TargetURL}, nil
}

// DeleteSearchRedirect removes a redirect.
func (r *mutationResolver) DeleteSearchRedirect(ctx context.Context, channelID string, id string) (bool, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	rid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", nil)
	}
	if err := r.SearchMerchSvc.DeleteRedirect(ctx, cid, rid); err != nil {
		return false, err
	}
	return true, nil
}

// SearchMerchandising returns all three rule sets for one channel.
func (r *queryResolver) SearchMerchandising(ctx context.Context, channelID string) (*model.SearchMerchandising, error) {
	cid, err := r.searchMerchChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	syns, err := r.SearchMerchSvc.ListSynonyms(ctx, cid)
	if err != nil {
		return nil, err
	}
	pins, err := r.SearchMerchSvc.ListPins(ctx, cid)
	if err != nil {
		return nil, err
	}
	boosts, err := r.SearchMerchSvc.ListBoosts(ctx, cid)
	if err != nil {
		return nil, err
	}
	reds, err := r.SearchMerchSvc.ListRedirects(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := &model.SearchMerchandising{
		Synonyms:  make([]*model.SearchSynonym, 0, len(syns)),
		Pins:      make([]*model.SearchPin, 0, len(pins)),
		Boosts:    make([]*model.SearchBoost, 0, len(boosts)),
		Redirects: make([]*model.SearchRedirect, 0, len(reds)),
	}
	for _, b := range boosts {
		out.Boosts = append(out.Boosts, &model.SearchBoost{ID: b.ID.String(), Query: b.Query, ProductID: b.ProductID.String(), Weight: b.Weight})
	}
	for _, s := range syns {
		out.Synonyms = append(out.Synonyms, &model.SearchSynonym{ID: s.ID.String(), Terms: s.Terms})
	}
	for _, p := range pins {
		out.Pins = append(out.Pins, &model.SearchPin{ID: p.ID.String(), Query: p.Query, ProductID: p.ProductID.String(), Position: p.Position})
	}
	for _, rd := range reds {
		out.Redirects = append(out.Redirects, &model.SearchRedirect{ID: rd.ID.String(), Query: rd.Query, TargetURL: rd.TargetURL})
	}
	return out, nil
}
