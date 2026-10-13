# Checkout delivery options

Checkout offers four ways to receive a physical order:

| Kind | Slug | What the shopper gives |
|---|---|---|
| Delivery to address | `ship_to_address` | A shipping address |
| Store pickup | `store_pickup` | A store, and who collects |
| Pickup point | `parcel_locker` | A locker, parcel shop or PO box from a list; its address becomes the shipping address |
| Nearest pickup point | `carrier_point` | An address; the carrier chooses a nearby pickup point and notifies the shopper |

Every order also asks for an email and a payment method. Digital-only carts skip delivery
and ask only for billing.

The kinds live in code (`internal/delivery/builtin.go`). Each one declares the blocks it
asks for and how far a merchant may relax each block:

- **Locked:** always required.
- **Configurable:** the merchant picks required, optional or not asked.
- **N/a:** never asked, because something else supplies it.

The backend turns the chosen deliveries into the cart's `requirements`. The storefront
renders the checkout from them, and `placeOrder` refuses while any requirement is missing.

## Configure a channel

**Settings → Checkout** is scoped to the selected channel.

1. **Overview** shows the go-live checklist. A channel with no saved options runs on the
   built-in two (Delivery and Store pickup). Choose **Start customising** to save them as
   the channel's own.
2. **Delivery** lists the channel's options. Add one by kind, and for each set:
   - its name
   - which configurable blocks are asked (phone, and billing for store pickup)
   - where its points come from
   - its **Carrier**: *Any carrier*, or one carrier whose methods of the kind it lists

   Stage **Offer at checkout** with the setup changes. Save checks readiness before enabling it, so a valid option can be configured and offered in one confirmed save.
3. **Pickup points** lists the lockers, parcel shops and PO boxes a pickup point option offers when its points
   come from this list (`manual-points`, the default for the kind).

Edits are staged and applied through the page-header Save, including edits on other tabs. Failed saves retain the draft for correction and retry.

## Where points come from

| Source | Serves | Configured in |
|---|---|---|
| `inventory-locations` | Store pickup | Inventory locations that are active, pickup-enabled and assigned to the channel |
| `manual-points` | PO box | Checkout → PO boxes |
| `remote:<slug>` | PO box | A `delivery_points` remote provider ([remote-providers.md](remote-providers.md)), for a locker network's API |

The backend re-reads a picked point from its source when it is chosen and again at
placement, so a PO box removed in the meantime is refused.

## What can be delivered how: product types

A product type's **Allowed delivery kinds** limit how its products can be delivered. Choose **All offered kinds** to allow every kind the channel offers, or **Selected kinds** to limit them. A fridge type allows *Delivery to
address* and *Store pickup*, so a fridge is never offered a PO box.

A cart is offered only the kinds every line allows. When its lines have no kind in
common, or the shopper asks to choose per group, the cart splits into delivery groups.
Each group has its own delivery, and each group is priced per package and origin.

## What delivery costs: shipping methods

A delivery option is priced by the carrier methods of its kind. The built-in **Shipping**
zones price door delivery and PO boxes: each method names the kind it delivers to.

A method's price is worked out per package:

- **Price**, charged once, or again for every started bundle of **Charge again every … items**.
  For example, 5 € every 5 items charges 5 € for 1-5 items and 10 € for 6-10.
- **Rates by product type** price that type's items on their own row instead of the
  method's price. For example, Fridge at 30 € every 1 item.
- **Free above** waives the method's own price once the cart reaches the threshold. A
  product type's rate is waived only when its **Can ship free** box is ticked, so a fridge
  keeps costing 30 € unless the merchant allows otherwise.

Example: a method at 5 € every 5 items, free above 100 €, and a Fridge rate of 30 € per
item that can't ship free.

| Cart | Below 100 € | 100 € or more |
|---|---|---|
| 3 t-shirts | 5 € | free |
| 6 t-shirts | 10 € | free |
| 3 t-shirts and a fridge | 35 € | 30 € |

Store pickup is the built-in pickup carrier and is always free.

## Several methods, carriers and options

Checkout offers every method of an option's kind from every enabled carrier, one choice
each. A single *Delivery* option therefore lists Standard, Express and Free side by side,
and a second carrier's methods beside the store's own. Each card names the option, then
the carrier when the option's methods come from more than one, then the method when the
option has more than one.

Add a second option of the same kind only to show a separately named entry, and give each
a different **Carrier**. Two options of one kind that can take the same carrier's methods
list each of those methods twice. The option's setup checklist warns about this, but the
warning doesn't stop the option being offered.

## Delivery dates

A method quotes delivery days, and checkout turns them into the date the shopper sees
("Arrives Mon 5 Oct"). The count starts the day after the order, on the calendar of
**Settings → General → Store information → Store timezone**. Saturday and Sunday are skipped unless the
carrier delivers on weekends, so a Friday order quoted 1 day arrives Monday, or Saturday
from a weekend carrier. The **Fastest** badge goes by that date, not by the days quoted.

| Carrier | Where the days and weekend rule are set |
|---|---|
| Built-in shipping methods | Each method's **Delivery days** (0 shows no date) and **Delivers on weekends** |
| Plugin carriers | The carrier quotes the days; **Delivers on weekends** is on its row in the Carriers list |
| Remote `shipping_carrier` providers | The provider returns `etaDays`; **Delivers on weekends** is on the provider, stored in its config as `"deliversOnWeekends": true` |

## Shopper delivery step

Delivery choices save immediately, but the step stays open until the shopper chooses
**Continue to details**. For pickup points, search by name, city or postcode; a selected
point remains visible when filtering. Source failures offer Retry separately from empty
search results. Split orders show their groups and combined selected delivery price.
A free-above threshold may leave product-specific charges; the quoted option price is
the delivery cost.

## Payment setup

The Payment tab owns per-channel availability, country rules, checkout labels and display
order, plus manual-payment instructions. Provider credentials and test/live settings live
under **Settings → Payment providers**, where installed gateway plugins appear automatically.
Those fields come from the plugin's configuration registry. Runtime-disabled plugins are
not offered at checkout, even if their payment-method toggle was previously on. All edits
use the page-header Save.
