# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `check` command: probes the SOCKS proxy, confirms traffic exits over Tor and
  prints the exit address.
- `newnym` command: requests a fresh Tor circuit through the control port.
- `version` command and `--version`, reporting the version, commit, build date,
  Go version and platform.
- `exec` now accepts a program and arguments directly
  (`go-toralizer exec -- curl https://example.com`), which runs without a shell.
- `--verbose`, which the README previously documented but the tool did not
  implement.
- `--quiet` to detach the command's output from the terminal, and `--skip-verify`
  to skip the connectivity check.
- `--shell` to choose the interpreter used for `--command`.
- Configuration from the environment: `TORALIZER_PROXY`, `TORALIZER_TIMEOUT`,
  `TORALIZER_CHECK_URL`, `TORALIZER_VERBOSE` and others.
- Distinct exit codes for a missing proxy (64), traffic not exiting over Tor
  (65), a timeout (66) and a control-port failure (67).
- Continuous integration: golangci-lint, `go vet`, tests under the race detector
  on Linux, macOS and Windows, cross-compilation, `govulncheck`, CodeQL and
  Dependabot.
- Releases built and published by GoReleaser, with checksums and SBOMs.

### Changed

- **Commands now actually go through Tor.** The proxy environment
  (`ALL_PROXY`, `HTTP_PROXY`, `HTTPS_PROXY` and their lower-case spellings) is
  exported to the child process; previously the command ran with the ambient
  environment and the proxy flag had no effect.
- **The proxy check is real.** `isTorProxyRunning` and `verifyTorConnection`
  returned `true` and `nil` unconditionally. They now perform a SOCKS5 greeting
  against the proxy and a request through it to a check endpoint.
- Exported proxy URLs use the `socks5h` scheme, so hostnames are resolved by the
  exit relay rather than by the local resolver.
- The command's exit status is passed through instead of being discarded.
- `--output` streams to the file as output is produced rather than buffering it
  all in memory, writes it with mode 0600, and no longer suppresses the terminal
  output.
- `SIGINT` and `SIGTERM` are forwarded to the command, which is given a grace
  period to exit before being killed.
- Upgraded to Go 1.27 and `urfave/cli` v3.
- The single `main.go` was split into `cmd/` and `internal/` packages.
- `go.sum` is committed rather than ignored, so builds are reproducible.

### Fixed

- A timeout no longer leaves the child process running.
- `--proxy` is validated, rather than being accepted and ignored.
- Diagnostics go to stderr, so they no longer mix into the command's output.
