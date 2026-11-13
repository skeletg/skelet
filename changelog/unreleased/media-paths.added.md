- **Where uploaded files are kept is a setting.** Settings, System, Media files holds four templates: how a
  library file is named inside its folder, and where product pictures, category covers and collection covers go
  under `catalog/`. They're written with placeholders, like `products/{sku}/{type}/{name}-{short_id}{ext}`,
  and each needs `{id}` or `{short_id}` so an address is never used twice. A template only applies to what is
  uploaded after it, plus library files that are moved or renamed.
