// The plugin registry lives in @skelet/storefront-react, so a plugin from npm registers into the same one this storefront
// reads. Importing it through here loads the installed plugins too, which the browser needs as much as the server:
// a slot renders in a client component, and that reads the browser's own copy of the registry
//
// The registry comes first. A plugin copied into app/plugins/ imports this file to register, which is a cycle through
// ./loaded - and in the other order its registration would run before the registry it calls exists
export * from "@skelet/storefront-react/plugins/registry";

import "./loaded";
