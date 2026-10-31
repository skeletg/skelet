# Your storefronts

Each storefront this project runs lives here, one directory per channel. Make one from a template:

```bash
skelet add channel shop --template nextjs-vetrina
```

That copies `templates/nextjs-vetrina` into `apps/channels/shop` and wires its compose service. From then on the copy
is yours to change - upstream keeps improving the templates and the `packages/` they're built on, and since you
never edit either, merging a new release doesn't collide with your storefront.
