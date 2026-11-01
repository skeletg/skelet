- **`skelet setup` draws its questions in the terminal.** Arrow keys pick the target, the template and the search
  engine, checkbox lists take the apps, dev tools and payment gateways, and each answer folds into one line under its
  section. Before anything is written a review lists every answer - enter goes ahead, `e` asks again from the answers
  given, `q` leaves with nothing written. The slow steps run behind a spinner, their output in `.skelet/setup.log`,
  shown when one fails. Without a terminal, or with `--yes`, `--profile` or `NO_COLOR`, setup asks in plain lines as
  before.
