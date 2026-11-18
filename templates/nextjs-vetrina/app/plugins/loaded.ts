// The installed storefront plugins, imported for their registration side effects
//
// A hand-maintained list rather than a build-time glob, for the reason the backend's
// plugins/firstparty is: the ENABLED set decides what loads, not whatever happens to be sitting in
// a directory. It is the one storefront file installing a plugin edits
//
// Imported by the root layout for the server, and by registry.ts and Slots.tsx for the browser, which keeps its own
// copy of the registry - so registration finishes before anything reads it on either side

// The enabled plugins' npm packages come first, generated from plugins.json by skelet gen - a plugin copied into
// app/plugins/<slug>/ is imported below by hand
import "./packages";

export {};
