# Security policy

## Reporting a vulnerability

Please report security issues privately through
[GitHub's private advisory form](https://github.com/zvdy/go-toralizer/security/advisories/new)
rather than by opening a public issue.

Include what the issue allows an attacker to do, the version you tested and the
steps to reproduce it. Leave out any real hostnames, exit addresses or captured
traffic.

## Supported versions

Fixes land on the latest release. Older tags are not patched.

## What this tool does and does not protect

`go-toralizer` sets proxy environment variables for the command it runs. That is
a cooperative mechanism, and it has limits worth understanding before relying
on it:

- **Programs that ignore the proxy environment are unaffected.** Anything that
  opens raw sockets, uses its own resolver, or does not read `ALL_PROXY` and
  friends will connect directly. `go-toralizer` cannot see this happen.
- **It is not a sandbox.** Use `torsocks`, a transparent proxy, or a network
  namespace when you need enforcement rather than cooperation.
- **DNS is routed through Tor**, because the exported URLs use the `socks5h`
  scheme, which asks the proxy to resolve names. A program that resolves names
  through its own resolver instead still leaks them.
- **`--command` runs through a shell.** Anything interpolated into it is shell
  input. Prefer the argument form, `go-toralizer exec -- program args...`,
  which involves no shell at all.
- **`--output` files are created with mode 0600**, but their contents are
  whatever the command produced.

## Out of scope

Deanonymisation attacks against the Tor network itself belong to
[the Tor Project](https://gitlab.torproject.org/tpo/core/tor/-/issues), not to
this repository.
