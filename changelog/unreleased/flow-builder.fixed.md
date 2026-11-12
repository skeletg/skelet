- **The delete key no longer removes a flow's trigger.** A flow can't be activated without one, and there was no
  way to add it back.
- **A flow can't be activated with a placeholder email.** A send-email step with no email picked would send the
  "Hello" placeholder it was created with. A new step now starts empty.
- **A flow that can't run can't be activated, whoever asks.** The server refuses a flow with nothing after its
  trigger, a loop, or a step on the way that's missing its email, message, segment or URL. The builder already
  held the button back, but a direct API call went through.
- **The flow builder says when its lists didn't load.** A failed load of the triggers, segments or emails shows
  under the Flow tab instead of leaving empty menus.
