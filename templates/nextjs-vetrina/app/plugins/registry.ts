// The plugin registry lives in @skelet/storefront-react, so a plugin from npm registers into the same one this storefront
// reads. Importing it through here loads the installed plugins first, which the browser needs as much as the server:
// a slot renders in a client component, and that reads the browser's own copy of the registry
import "./loaded";

export * from "@skelet/storefront-react/plugins/registry";
