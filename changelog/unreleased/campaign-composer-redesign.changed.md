- **A campaign is written as a sentence.** The new campaign page reads "Send *this email* to *these people*
  *now*", and each underlined part opens the control that sets it: the library's emails by name and subject,
  everyone, a customer group or a segment, and now or a set time. An A/B test adds a second sentence for the
  other subject, the share of the audience, how long the test runs and what decides the winner.
- **The page shows what a send will do before it goes.** The email as it lands in the inbox, with both subjects
  for an A/B test, above the opened email. How many of the audience are opted in, and how an A/B test deals
  them out. And when each part goes out. Send now or Schedule then asks once, repeating the sentence.
- A scheduled, sending, sent or cancelled campaign opens on the same sentence.
- **A draft remembers when it plans to go out.** The time picked in the sentence is saved with the draft
  (`scheduledFor` on `SaveCampaignInput`) and is there when the draft is opened again. Nothing is sent until the
  campaign is scheduled.
- **The share an A/B test may take comes from the server.** The admin API's new `campaignLimits` carries the
  bounds, so the slider and the save can't disagree.
