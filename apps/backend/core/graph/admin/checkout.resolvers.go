package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

func (r *mutationResolver) CreateDeliveryOption(ctx context.Context, channelID string, input model.DeliveryOptionInput) (*model.DeliveryOption, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel", err)
	}
	if err := r.requireCheckout(ctx, chID, true); err != nil {
		return nil, err
	}
	in, err := toOptionInput(input)
	if err != nil {
		return nil, err
	}
	o, err := r.DeliverySvc.CreateOption(ctx, chID, in)
	if err != nil {
		return nil, err
	}
	return r.deliveryOptionModel(ctx, o, false), nil
}

func (r *mutationResolver) UpdateDeliveryOption(ctx context.Context, id string, input model.DeliveryOptionInput) (*model.DeliveryOption, error) {
	oid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("delivery_option", err)
	}
	if _, err := r.requireOptionScope(ctx, oid); err != nil {
		return nil, err
	}
	in, err := toOptionInput(input)
	if err != nil {
		return nil, err
	}
	o, err := r.DeliverySvc.UpdateOption(ctx, oid, in)
	if err != nil {
		return nil, err
	}
	return r.deliveryOptionModel(ctx, o, false), nil
}

func (r *mutationResolver) DeleteDeliveryOption(ctx context.Context, id string) (bool, error) {
	oid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("delivery_option", err)
	}
	if _, err := r.requireOptionScope(ctx, oid); err != nil {
		return false, err
	}
	if err := r.DeliverySvc.DeleteOption(ctx, oid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AdoptDefaultDeliveryOptions(ctx context.Context, channelID string) (*model.CheckoutConfiguration, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel", err)
	}
	if err := r.requireCheckout(ctx, chID, true); err != nil {
		return nil, err
	}
	if err := r.DeliverySvc.AdoptDefaults(ctx, chID); err != nil {
		return nil, err
	}
	return r.checkoutConfiguration(ctx, chID)
}

func (r *mutationResolver) SaveDeliveryPoints(ctx context.Context, channelID string, points []*model.DeliveryPointInput) ([]*model.DeliveryPointAdmin, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel", err)
	}
	if err := r.requireCheckout(ctx, chID, true); err != nil {
		return nil, err
	}
	in, err := toManualPoints(points)
	if err != nil {
		return nil, err
	}
	saved, err := r.DeliverySvc.SavePoints(ctx, chID, in)
	if err != nil {
		return nil, err
	}
	return toDeliveryPointModels(saved), nil
}

func (r *mutationResolver) SetProductTypeDeliveryKinds(ctx context.Context, productTypeID string, kinds []string) (*model.ProductType, error) {
	if err := r.requireGlobal(ctx, "catalog.manage"); err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(productTypeID)
	if err != nil {
		return nil, errInvalidID("product_type", err)
	}
	if err := r.DeliverySvc.SetProductTypeKinds(ctx, pid, kinds); err != nil {
		return nil, err
	}
	pt, err := r.CatalogSvc.GetProductType(ctx, pid)
	if err != nil {
		return nil, err
	}
	return toProductTypeModel(pt), nil
}

func (r *orderResolver) Shipments(ctx context.Context, obj *model.Order) ([]*model.OrderShipment, error) {
	oid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, errInvalidID("order", err)
	}
	ships, err := r.OrderSvc.ListShipments(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.OrderShipment, len(ships))
	for i, s := range ships {
		out[i] = toOrderShipmentModel(s)
	}
	return out, nil
}

func (r *queryResolver) CheckoutConfiguration(ctx context.Context, channelID string) (*model.CheckoutConfiguration, error) {
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel", err)
	}
	if err := r.requireCheckout(ctx, chID, false); err != nil {
		return nil, err
	}
	return r.checkoutConfiguration(ctx, chID)
}
