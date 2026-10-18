# Security policy

Skelet takes payments, stores customer data and runs admin sessions, so we take
reports seriously and would much rather hear about a problem privately than read
about it.

## Reporting a vulnerability

**Please don't open a public issue, discussion or pull request for a security
problem.** Report it privately instead:

1. Go to the repository's **Security** tab.
2. Choose **Report a vulnerability** and fill in the form.

Only the maintainers can see the report, and we can work on a fix together in a
private advisory before anything is public.

A useful report has:

- what an attacker can do, and what they need first (an account, an admin role,
  network access to an internal service)
- the affected version or commit, and which part: core, Signals, the plugin SDK,
  the dashboard or a storefront
- steps or a small proof of concept that reproduces it
- anything you already know about a fix