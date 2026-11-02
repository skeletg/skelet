# Contributing to Skelet

Thanks for your interest. Skelet is still finding its shape, so for now we're
**not accepting pull requests** - code changes come from the maintainers while the
foundations settle.

What helps most right now is **issues**.

## Opening an issue

- **Bug** - something doesn't work the way it should. Say what you did, what you
  expected and what happened, and include the version or commit you're on. Steps
  that reproduce it on a fresh `skelet setup` are the most useful thing you can
  give us.
- **Idea** - something Skelet should do. Start with the problem you're trying to
  solve rather than the solution you have in mind; it often turns out a plugin can
  cover it without a change to core.

Please search the existing issues first, and keep one problem per issue.

**Found a security problem?** Don't open an issue - report it privately as
described in [SECURITY.md](SECURITY.md).

## Building your own extensions

You don't need a pull request to extend Skelet. Plugins, remote providers,
storefronts, dashboard extensions and themes live in your own repositories, and
the plugin exception in [PLUGIN-EXCEPTION.txt](PLUGIN-EXCEPTION.txt) means they can
carry whatever licence you pick. [docs/plugins.md](docs/plugins.md) is the place to
start, and an issue is the right way to ask for an extension point that's missing.

## Running it yourself

You need Docker; everything else runs in containers. The front door is the `skelet`
CLI - install it the way the README shows, and in a clone it runs that clone's own code.

```bash
skelet setup     # picks apps, writes env files, migrates, boots
skelet           # lists every command
```

[AGENTS.md](AGENTS.md) is the map of the repo, and [docs/README.md](docs/README.md)
indexes the rest of the documentation.
