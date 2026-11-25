- **Coupons and promotions leave a set price alone.** A line whose price an admin set - on
  an offer, a custom line, a shopper's cart - is no longer discounted further by default: the
  price is the deal. It still counts toward what the cart holds, so a spend threshold is
  reached with it and the discount comes off the other lines. Whoever sets the price can let
  promotions apply (`promotionsApply` on `adminSetCartLinePrice` and on a custom line's
  input), cart and order lines say which it was, and both storefront templates tell the
  shopper. Plugin promotion actions see `PromotionCartLine.NoDiscount`, and a remote
  promotion provider `noDiscount` on the line; a discount named for such a line is dropped.
