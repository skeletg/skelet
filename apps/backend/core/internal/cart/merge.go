package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"
)

// Sums by SKU with the guest's unit_price winning, and with no active cart the guest cart is adopted outright - idempotent, so a guest cart already non-active just returns the current active cart
func (s *Service) MergeGuestCart(ctx context.Context, customerID, guestCartID uuid.UUID) (Cart, error) {
	var resultID uuid.UUID
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)

		guest, err := r.GetCartByID(ctx, guestCartID)
		if err != nil {
			if IsSQLNoRows(err) {
				return ErrCartForbidden
			}
			return fmt.Errorf("load guest cart: %w", err)
		}
		if guest.CustomerID != nil || guest.Kind == KindOffer { // another customer's cart answers like a missing one, so the id can't be probed - and an offer stays a side cart, it never folds into the shopper's
			return fmt.Errorf("%w: not a guest cart", ErrCartForbidden)
		}

		if guest.Status != "active" { // already consumed, so hand back the customer's active cart, creating one if needed
			cc, err := GetOrCreateCustomerCart(ctx, r, customerID, guest.ChannelID)
			if err != nil {
				return err
			}
			resultID = cc.ID
			return nil
		}

		cc, err := r.GetCartByCustomerAndChannel(ctx, customerID, guest.ChannelID) // no active cart on this store takes the adoption path below, repurposing the guest cart - cross-store merges are rejected
		if err != nil {
			if !IsSQLNoRows(err) {
				return err
			}
			adopted, err := r.ReassignCart(ctx, guest.ID, customerID)
			if err != nil {
				return err
			}
			resultID = adopted.ID
			return nil
		}
		if cc.ChannelID != guest.ChannelID {
			return ErrCartStoreMismatch
		}

		if s.bus != nil { // sync pre-commit hook, so a plugin can reject the merge - the bus inside the tx is fine, since subscribers run on the caller's goroutine with no detached fan-out
			if err := s.bus.PublishSync(ctx, &Merging{
				GuestCartID:    guest.ID,
				CustomerCartID: cc.ID,
				CustomerID:     customerID,
				ChannelID:      cc.ChannelID,
			}); err != nil {
				return fmt.Errorf("merge rejected: %w", err)
			}
		}

		if err := r.SetCartMerged(ctx, guest.ID); err != nil { // flipping status FIRST is the serialization point against a concurrent second merge - losing the race skips the upserts and returns the cart as-is
			if errors.Is(err, ErrCartAlreadyConverted) {
				resultID = cc.ID
				return nil
			}
			return err
		}
		guestLines, err := r.ListCartLinesByCart(ctx, guest.ID)
		if err != nil {
			return err
		}
		if err := copyLines(ctx, r, cc.ID, guestLines); err != nil {
			return err
		}
		saved, err := r.ListSavedItems(ctx, guest.ID) // saved-for-later comes along too - the adoption path keeps them via the same cart_id, but merging into an existing cart would drop them when the guest cart is tombstoned; the upsert dedupes by (cart_id, sku)
		if err != nil {
			return err
		}
		for _, si := range saved {
			if _, err := r.SaveItem(ctx, cc.ID, si.ProductSku, si.ProductName, si.Quantity); err != nil {
				return err
			}
		}
		if err := r.SetCheckout(ctx, cc.ID, mergeCheckout(guest.Checkout, cc.Checkout)); err != nil { // the adoption path keeps checkout state on the same row; a merge carries it across
			return err
		}
		if guest.Email != "" { // the receipt address the guest typed wins, like the rest of their checkout
			if err := r.SetEmail(ctx, cc.ID, guest.Email); err != nil {
				return err
			}
		}
		resultID = cc.ID
		return nil
	})
	if err != nil {
		return Cart{}, err
	}
	if s.bus != nil {
		merged, lookupErr := s.repo.GetCartByID(ctx, resultID) // the tx has committed, and resultID is either the adopted guest cart or the customer's existing one
		if lookupErr == nil {
			s.bus.Publish(ctx, &Merged{
				CustomerCartID: resultID,
				GuestCartID:    guestCartID,
				CustomerID:     customerID,
				ChannelID:      merged.ChannelID,
			})
		}
	}
	return loadCartWithLines(ctx, s.repo, resultID)
}

// Copies lines onto a cart that may already hold some - bundle and configured lines keep their snapshot rows (the unit price already includes their deltas), plain lines sum
// The line already there wins any collision a copy would corrupt: a plain upsert onto a configured line sums quantity and resets the price with the option rows still attached, and a bundle or configured upsert replaces the picks already there
func copyLines(ctx context.Context, r Repository, destID uuid.UUID, lines []CartLine) error {
	existing, err := r.ListCartLinesByCart(ctx, destID)
	if err != nil {
		return fmt.Errorf("load destination lines: %w", err)
	}
	plain := make(map[string]bool)
	snapshotted := make(map[string]bool)
	for _, el := range existing {
		if len(el.Options) > 0 || len(el.Components) > 0 {
			snapshotted[el.ProductSku] = true
		} else {
			plain[el.ProductSku] = true
		}
	}
	for _, l := range lines {
		if snapshotted[l.ProductSku] {
			continue
		}
		var err error
		switch {
		case len(l.Components) > 0:
			if plain[l.ProductSku] {
				continue
			}
			_, err = r.UpsertBundleCartLine(ctx, destID, AddBundleLineInput{
				ProductName: l.ProductName,
				ProductSku:  l.ProductSku,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
				Components:  componentInputs(l.Components),
				HookPriced:  l.BundleHookPriced,
			})
		case len(l.Options) > 0:
			if plain[l.ProductSku] {
				continue
			}
			_, err = r.UpsertConfiguredCartLine(ctx, destID, AddConfiguredLineInput{
				ProductName: l.ProductName,
				ProductSku:  l.ProductSku,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
				Options:     l.Options,
			})
		default:
			_, err = r.UpsertCartLineBySku(ctx, destID, AddCartLineInput{
				ProductName: l.ProductName,
				ProductSku:  l.ProductSku,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
			})
		}
		if err != nil {
			return fmt.Errorf("copy line %q: %w", l.ProductSku, err)
		}
	}
	return nil
}

// mergeCheckout is what a customer cart holds after a guest cart merges into it - the guest's selections win, since that is what the shopper was just filling in, and the customer cart's fill only what the guest left empty
func mergeCheckout(guest, customer Checkout) Checkout {
	out := customer
	if guest.ShippingAddress != nil {
		out.ShippingAddress, out.ShippingAddressID = guest.ShippingAddress, guest.ShippingAddressID
	}
	if guest.ShippingAddress != nil || guest.BillingAddress != nil { // billing travels as a unit with its same-as-shipping flag
		out.BillingSameAsShipping = guest.BillingSameAsShipping
		out.BillingAddress, out.BillingAddressID = guest.BillingAddress, guest.BillingAddressID
	}
	if guest.ShippingCarrierSlug != "" {
		out.ShippingCarrierSlug, out.ShippingMethodCode = guest.ShippingCarrierSlug, guest.ShippingMethodCode
	}
	if guest.PaymentProviderSlug != "" {
		out.PaymentProviderSlug = guest.PaymentProviderSlug
	}
	if guest.PONumber != "" {
		out.PONumber = guest.PONumber
	}
	if !guest.Destination.IsZero() {
		out.Destination = guest.Destination
	}
	if guest.ContactPhone != "" {
		out.ContactPhone = guest.ContactPhone
	}
	if guest.PurchaseType == PurchaseBusiness { // the business details travel as a unit with the purchase type
		out.PurchaseType, out.Company, out.BusinessIDs = guest.PurchaseType, guest.Company, guest.BusinessIDs
	}
	if guest.PickupContact != nil {
		out.PickupContact = guest.PickupContact
	}
	if len(guest.Deliveries) > 0 { // the guest's delivery choices win per group, and the split they chose them under travels with them
		out.Deliveries = maps.Clone(customer.Deliveries)
		if out.Deliveries == nil {
			out.Deliveries = map[string]DeliverySelection{}
		}
		maps.Copy(out.Deliveries, guest.Deliveries)
		out.SplitDelivery = guest.SplitDelivery
	}
	out.UseStoreCredit = customer.UseStoreCredit // a guest can't hold store credit
	seen := make(map[string]bool)
	out.GiftCardCodes = nil
	for _, c := range append(append([]string(nil), guest.GiftCardCodes...), customer.GiftCardCodes...) {
		if !seen[c] {
			seen[c] = true
			out.GiftCardCodes = append(out.GiftCardCodes, c)
		}
	}
	return out
}
