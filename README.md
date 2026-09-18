# go-toralizer

[![CI](https://github.com/zvdy/go-toralizer/actions/workflows/ci.yml/badge.svg)](https://github.com/zvdy/go-toralizer/actions/workflows/ci.yml)
[![CodeQL](https://github.com/zvdy/go-toralizer/actions/workflows/codeql.yml/badge.svg)](https://github.com/zvdy/go-toralizer/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zvdy/go-toralizer)](https://goreportcard.com/report/github.com/zvdy/go-toralizer)
[![Go Reference](https://pkg.go.dev/badge/github.com/zvdy/go-toralizer.svg)](https://pkg.go.dev/github.com/zvdy/go-toralizer)
[![Go](https://img.shields.io/badge/Go-1.27-%2300ADD8.svg?&logo=go&logoColor=white)](go.mod)
[![Tor](https://img.shields.io/badge/Tor-7D4698?logo=Tor-Browser&logoColor=white)](https://www.torproject.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`go-toralizer` runs a command with its traffic routed through your local Tor
SOCKS proxy. Before the command starts it checks that Tor is actually there and
that traffic through it really leaves the Tor network, so a misconfigured daemon
fails loudly instead of quietly sending your request in the clear.

```console
$ go-toralizer exec -- curl -s https://check.torproject.org/api/ip
{"IsTor":true,"IP":"185.220.101.1"}
```

## How it works, and what that means

`go-toralizer` exports proxy environment variables to the command it runs:

| Variable                              | Value                        |
| ------------------------------------- | ---------------------------- |
| `ALL_PROXY`, `all_proxy`              | `socks5h://127.0.0.1:9050`   |
| `HTTP_PROXY`, `http_proxy`            | `socks5h://127.0.0.1:9050`   |
| `HTTPS_PROXY`, `https_proxy`          | `socks5h://127.0.0.1:9050`   |
| `TORALIZER`                           | `1`                          |

The `socks5h` scheme, rather than `socks5`, tells the client to let the proxy
resolve hostnames. That keeps DNS lookups inside Tor; with plain `socks5` the
program would resolve names with your local resolver and leak every hostname it
visits.

This is a cooperative mechanism, and it is worth being clear about the limits:

> [!WARNING]
> **`go-toralizer` is not a sandbox.** A program that ignores the proxy
> environment — anything using raw sockets or its own resolver — connects
> directly, and `go-toralizer` cannot tell that it did. If you need enforcement
> rather than cooperation, use [`torsocks`](https://gitweb.torproject.org/torsocks.git),
> a transparent proxy, or a network namespace.

See [SECURITY.md](SECURITY.md) for the full list of caveats.

## Requirements

- A running Tor daemon with its SOCKS port reachable (`127.0.0.1:9050` by
  default). On Debian and Ubuntu: `sudo apt install tor && sudo systemctl start tor`.
- Go 1.27 or newer, if you are building from source.

## Installation

```sh
go install github.com/zvdy/go-toralizer/cmd/go-toralizer@latest
```

Or from a clone, which stamps the version into the binary:

```sh
git clone https://github.com/zvdy/go-toralizer.git
cd go-toralizer
make build
sudo install -m 0755 bin/go-toralizer /usr/local/bin/
```

Pre-built binaries for Linux, macOS and Windows are attached to each
[release](https://github.com/zvdy/go-toralizer/releases), with checksums and an
SBOM.

## Usage

```
go-toralizer [global options] command [command options] [arguments...]

COMMANDS:
   exec, run      run a command with its traffic routed through Tor
   check          verify that the Tor proxy is reachable and traffic exits over Tor
   newnym, renew  request a fresh Tor circuit via the control port
   version        print build information
```

### Running a command

The argument form is preferred. Nothing is word-split or expanded between you
and the program, because no shell is involved:

```sh
go-toralizer exec -- curl -s https://example.com
```

Use `--command` when you want a pipeline, a redirection or shell expansion:

```sh
go-toralizer exec --command "curl -s https://example.com | head -n 20"
```

> [!CAUTION]
> `--command` is shell input. Never interpolate untrusted values into it; use
> the argument form instead.

### Checking your setup

```sh
$ go-toralizer check
proxy:  socks5://127.0.0.1:9050 (reachable)
tor:    yes
exit:   185.220.101.1
```

### Getting a new exit address

Requires `ControlPort` in your `torrc`:

```sh
go-toralizer newnym --control-password "$TOR_CONTROL_PASSWORD"
```

### Common options

| Flag                        | Default                                  | Description                                            |
| --------------------------- | ---------------------------------------- | ------------------------------------------------------ |
| `--proxy`, `-p`             | `socks5://127.0.0.1:9050`                | Tor SOCKS proxy address                                |
| `--verbose`, `-V`           | off                                      | Progress and diagnostics on stderr                     |
| `--check-url`               | `https://check.torproject.org/api/ip`    | Endpoint used to confirm traffic exits over Tor        |
| `--dial-timeout`            | `5s`                                     | How long to wait for the proxy to accept a connection  |
| `--check-timeout`           | `30s`                                    | How long to wait for the connectivity check            |
| `exec --timeout`, `-t`      | `30s`                                    | Abort the command after this long; `0` disables it     |
| `exec --output`, `-o`       | —                                        | Also write the command's output to a file (mode `0600`) |
| `exec --quiet`, `-q`        | off                                      | Suppress the command's output on this terminal         |
| `exec --shell`              | `sh -c` (`cmd /C` on Windows)            | Interpreter used for `--command`, repeat per argument  |
| `exec --skip-verify`        | off                                      | Only check that the proxy is listening                 |

Every global flag can also be set from the environment: `TORALIZER_PROXY`,
`TORALIZER_TIMEOUT`, `TORALIZER_CHECK_URL`, `TORALIZER_CHECK_TIMEOUT`,
`TORALIZER_DIAL_TIMEOUT`, `TORALIZER_VERBOSE`, `TORALIZER_SKIP_VERIFY`,
`TORALIZER_CONTROL_ADDRESS`, `TORALIZER_CONTROL_PASSWORD`.

### Exit codes

The command's own exit status is passed through unchanged. `go-toralizer`'s own
failures use codes above the range programs normally return, so you can tell
them apart:

| Code   | Meaning                                              |
| ------ | ---------------------------------------------------- |
| `0`    | The command ran and exited zero                      |
| `1`    | A `go-toralizer` failure                             |
| `2`    | The arguments were rejected                          |
| `64`   | No Tor SOCKS proxy answered                          |
| `65`   | The proxy answered, but traffic did not exit over Tor |
| `66`   | The command exceeded its timeout                     |
| `67`   | A Tor control-port operation failed                  |
| *other*| The command's own exit status                        |

```sh
if ! go-toralizer check; then
  case $? in
    64) echo "start tor first" ;;
    65) echo "tor is running but traffic is not going through it" ;;
  esac
fi
```

### A shell alias

```sh
# ~/.bashrc or ~/.zshrc
toralize() {
  go-toralizer exec -- "$@"
}

# then:
toralize curl -s https://example.com
```

## Development

```sh
make help       # list every target
make ci         # lint, vet and tests under the race detector
make cover      # coverage report
```

The test suite needs no Tor daemon: `internal/tor` stands up a SOCKS5 server
inside the test process, and `internal/runner` re-executes the test binary as a
helper program rather than assuming which utilities exist on the host.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the layout and the conventions.

## License

MIT. See [LICENSE](LICENSE).
