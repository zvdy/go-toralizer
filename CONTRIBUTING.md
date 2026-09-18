# Contributing to go-toralizer

Thanks for taking the time. This document covers what you need to build, test
and submit a change.

## Requirements

- Go 1.27 or newer. The module's `go` directive is the source of truth; `make`
  targets and CI both read it.
- A running Tor daemon, if you want to exercise the tool end to end. The test
  suite does not need one — it stands up an in-process SOCKS5 server instead.
- GNU Make, for the shortcuts below. Everything it does is a plain `go`
  command, so you can skip it if you prefer.

## Getting started

```sh
git clone https://github.com/zvdy/go-toralizer.git
cd go-toralizer
make build      # builds ./bin/go-toralizer
make ci         # lint + vet + tests under the race detector
```

`make help` lists every target.

## Layout

```
cmd/go-toralizer/     entry point: signal handling and exit codes
internal/app/         CLI definition, flags and command wiring
internal/tor/         SOCKS proxy parsing, probing, verification, control port
internal/runner/      child process execution
internal/buildinfo/   version details stamped in at build time
```

The side-effecting operations live behind the `app.Dependencies` struct, so the
CLI can be tested without a Tor daemon or a real child process. When you add a
command that talks to the network, add it there too rather than calling into
the network from the action directly.

## Before you open a pull request

```sh
make fmt        # gofmt
make lint       # golangci-lint, with the repository's configuration
make test-race  # the full suite under -race
make tidy       # go.mod and go.sum
```

CI runs the same checks on Linux, macOS and Windows, plus `govulncheck` and
CodeQL. A change that only passes on one platform will be caught there, so it
is worth keeping platform-specific code behind build tags — see
`internal/runner/signal_*.go` for the pattern.

## Tests

- Table-driven tests with named cases are the default.
- Prefer `t.Parallel()`; tests that set environment variables must not use it.
- Do not reach the real network. `internal/tor` starts a SOCKS5 server in the
  test process, and `internal/runner` re-executes the test binary as a helper
  program instead of assuming which shell utilities exist on the host.
- New behaviour needs a test. Bug fixes need a test that fails without the fix.

## Commit messages

[Conventional Commits](https://www.conventionalcommits.org/), because the
release notes are generated from them:

```
feat(exec): add --quiet to suppress command output
fix(tor): reject a non-SOCKS service listening on the proxy port
docs: explain the difference between socks5 and socks5h
```

Types in use: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`,
`chore`.

## Changes that affect anonymity

This tool exists to keep traffic inside Tor. If your change alters what leaves
the machine — the exported environment, the DNS path, what gets written to disk
— say so explicitly in the pull request, and add a test that pins the new
behaviour down. `TestProxyEnvURLAlwaysResolvesRemotely` is the model: it exists
solely to stop `socks5h` quietly becoming `socks5` again.

## Releases

Maintainers tag `vX.Y.Z` on `main`. The release workflow runs the tests, then
GoReleaser builds the archives, checksums and SBOMs and publishes the release.
