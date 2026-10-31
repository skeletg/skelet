- **`skelet up`'s port check stops when its input ends.** Ctrl-D at its prompt asked the same
  question again, forever.
- **Two services moved off busy ports in one go can't land on the same port.** The second was
  offered, or allowed, the port the first had just moved to, and compose then failed to bind it.
- **env/signals.env gets the value core runs with when env/go.env names a key twice.** It copied
  the first line, while compose reads the last.
- **Re-running `skelet up` leaves an unchanged stack alone.** Every `--build` gave the images a new
  ID through buildx's default provenance attestation, so the app, Signals and Docker-run frontends
  were recreated on every run. The dev images are built without it now; set
  `BUILDX_NO_DEFAULT_ATTESTATIONS` yourself to choose otherwise.
