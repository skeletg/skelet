// The installed storefront plugins, imported for their registration side effects
//
// A hand-maintained list rather than a build-time glob, for the reason the backend's
// plugins/firstparty is: the ENABLED set decides what loads, not whatever happens to be sitting in
// a directory. It is the one storefront file installing a plugin edits
//
// Imported by the root layout so registration finishes before any page renders

export {};
