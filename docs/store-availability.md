# Store availability

Enable **Settings → Sales → Store availability** globally or for a channel. It is
off by default. Under **Inventory locations**, mark each customer-facing location
**List in store finder**, activate it, and assign it to the intended channels.
Only **Retail store** locations can be publicly listed or accept online pickup.
A listed retail store remains visible when online pickup is disabled: walk-in
shopping and online collection are independent services.

Choose a structured type: Retail store, Warehouse, Storage, Dropshipper, or
Fulfillment partner. Unknown legacy classifiers become **Needs classification**,
except pickup points, which become Retail store so click-and-collect keeps working;
the previous text is retained in metadata as `_legacy_location_type`. The upgrade
removes public listing and pickup from locations explicitly typed as non-retail.
Inventory is retained.

The location editor groups the two services under **Customer access**, explains
the resulting behavior, and stages service removal when a store changes type.
Both services save atomically with the location. The admin table supports filters
for location type, city, status, public listing, and online pickup. Customers can
filter by **Pickup available** when a store offers it, independently of stock.
The storefront reserves `/stores` for the directory; do not use that path for CMS pages.

Customers can browse stores, filter by city or address, check a simple product or a
selected variant, show only stores with stock, request nearby sorting, and save a
preferred store on their browser. Location access happens only after clicking
**Near me**, and customer coordinates are not stored. Directions open Google Maps.
Store opening hours are displayed as a weekly schedule in the location's timezone.

Stock statuses use native per-location levels, subtracting outstanding reservations
for tracked inventory. Missing levels mean **Availability unknown**, not zero stock.
Online backorders and aggregate external stock do not establish store availability.
Low stock uses the product's positive low-stock threshold. Availability requires
enough stock for the smallest valid order quantity. Exact quantities are private.
Bundle/grouped/kit availability and additional configured components are not covered.
Checking stock does not reserve it or change checkout.

Flat inventory mode retains the public store directory but hides stock checking.
Disabling the feature hides navigation and product entry points, makes `/stores`
return not found, and prevents the public API from returning stores.

## Demo fixtures

On a development database only, run:

```bash
skelet cli dev-store-demo <vetrina-channel-id>
```

This adds named demo stores in Belgrade, Novi Sad and Niš, plus a private warehouse,
and enables the feature **only for the selected channel**. Re-running preserves
existing demo quantities and any manual changes; existing channel assignments are
retained. The demo warehouse is always kept private and classified as a warehouse.
It refuses flat inventory mode and never runs automatically on deploy.
The locations and contact details are fictional demonstration data.

| URL | What to check |
|---|---|
| `/store-demo-backpack` | In stock in Belgrade and Novi Sad; unknown in Niš |
| `/store-demo-scarf` | Low stock in each public store |
| `/store-demo-mug` | Out of stock publicly despite private warehouse stock |
| `/store-demo-fabric` | Fractional stock, sold in increments of 0.25 |
| `/store-demo-hoodie` | Small in Belgrade; Large in Novi Sad |
| `/stores` | Directory, product search, filtering, preferred store and nearby sorting |

Run the read-only browser journeys after installing the fixtures:

```bash
skelet test storefront vetrina e2e/stores.spec.ts
```
