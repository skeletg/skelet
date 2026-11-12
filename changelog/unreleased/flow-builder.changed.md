- **The flow builder has the email editor's layout.** Steps sit in a rail on the left, as icons on a narrow
  screen and labelled tiles on a wide one. The panel on the right has a Step tab for the selected step and a
  Flow tab for the name, what starts the flow, its status and its checks, with undo, Save and Activate pinned
  below it. Zoom, tidy up, the step count and the checks sit above the canvas.
- **A dot on a step leads one way.** Connecting a dot that already led somewhere moves the connection, since a
  flow only ever follows the first.
- **The transactional switch follows the trigger.** It turns on with an order or account trigger and off with
  any other, where it can no longer be switched on only for the save to be refused.
- **`flowTriggers` replaces `flowTriggerTypes` in the admin API.** Each trigger now comes with whether its flows
  may be transactional and whether they carry a one-time link, so the builder asks instead of keeping its own
  copy of those lists.
