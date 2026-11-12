- **Removing a storefront no longer leaves the dashboard's previews on its port.** `skelet add channel` points
  the previews at the project's first storefront (`VITE_STOREFRONT_ORIGIN` in `.env`), and `skelet rm channel`
  left that behind, so the CMS and blog previews showed "localhost refused to connect". They now move to the
  next storefront of the project's own, or back to the default. A value set by hand is left alone.
