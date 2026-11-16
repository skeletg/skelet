- **Invoices, packing slips and credit memos carry your details.** A channel sets the logo, the seller lines under the
  store name (legal name, address, tax and registry numbers) and each document's footer under Invoices and packing
  slips, and a country can add a note to the invoices of business purchases billed there, like a reverse-charge
  line. Each order keeps the seller lines, footers and country note it was placed under, so changing them never rewrites
  an old invoice.
  Invoices now print the buyer's company IDs and PO number too.
- **Plugins can make the documents themselves.** `RegisterDocumentRenderer` hands a plugin the order, the seller's
  details and core's own PDF, for a format of its own or a stamp on core's.
