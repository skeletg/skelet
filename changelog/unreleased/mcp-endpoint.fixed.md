- **The AI chat's product search reaches the whole catalog.** It used to scan only the first thousand
  products, so on a bigger store it answered "no products match" for things you sell. It now
  searches in the database, by name, SKU or URL, like the Products list does, newest first.
- **The AI chat's tools answer for the chat's channel.** A product's review counts and stock by
  location covered every channel, and review counts with the dashboard set to "All channels" did too.
- **A failed read in the AI chat says so.** "Could not load ..." came back to the model as an
  ordinary answer, and the cause was never logged. It's flagged as an error now, and logged.
- **The AI chat's overview tools say when a read failed.** Sales, web traffic, customer behaviour
  and experiments answered "no data" when the database or the Signals service couldn't be read, and
  the chat's opening snapshot left the section out the same way. They flag the failure now, and the
  snapshot says the section couldn't be read.
- **Money in the AI chat uses each currency's own decimals.** Every amount was printed with two, so
  yen came out a hundred times too small and dinars ten times too large. Promotion totals now name
  their currency too.
- **The AI chat's low-stock list covers the channel's own warehouses.** It filtered a store-wide page
  of 500 items down to the channel's, so a busy store could show "no low-stock items" for a channel
  that had some, and in flat inventory mode it never showed any. The insights card's count is the
  channel's now too.
- **The AI chat's window tops out at a year.** A larger window reached the sales queries unclamped.
