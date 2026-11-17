"use client";

// Through here rather than the package, so the installed plugins have registered before a zone renders - see registry.ts
import "./loaded";

export { Slots } from "@skelet/storefront-react/plugins/slots";
