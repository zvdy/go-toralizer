# go-toralizer

[![CI](https://github.com/zvdy/go-toralizer/actions/workflows/ci.yml/badge.svg)](https://github.com/zvdy/go-toralizer/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zvdy/go-toralizer)](https://goreportcard.com/report/github.com/zvdy/go-toralizer)

`go-toralizer` runs a command with its network traffic routed through a local Tor SOCKS proxy.

## Install

```sh
go install github.com/zvdy/go-toralizer/cmd/go-toralizer@latest
```

Or build from source with `make build`. Release binaries are signed with cosign and ship with an SBOM and build provenance attestation.

Requires a running Tor daemon (default `127.0.0.1:9050`; Tor Browser listens on `9150`).

## Usage

```sh
# argv form: no shell involved
go-toralizer exec -- curl https://check.torproject.org/api/ip

# shell form
go-toralizer exec --command "curl -s https://example.com | head"
```

| Flag | Default | Description |
| --- | --- | --- |
| `--command`, `-c` | | Shell command line; alternative to argv after `--` |
| `--proxy`, `-p` | `socks5h://127.0.0.1:9050` | Tor SOCKS5 proxy (`socks5` is upgraded to `socks5h`) |
| `--timeout`, `-t` | `30s` | Kill the command after this duration (`0` disables) |
| `--output`, `-o` | | Also write output to a file (created `0600`) |
| `--mode` | `auto` | `auto`, `env` or `torsocks` |
| `--skip-verify` | `false` | Skip the check.torproject.org exit verification |
| `--verbose`, `-v` | `false` | Log execution details to stderr |

The command's exit code is propagated.

## How routing works

Before running, the proxy is checked with a SOCKS5 handshake and, unless `--skip-verify` is set, a request through it must be confirmed by `check.torproject.org` as a Tor exit.

- `torsocks`: the command is wrapped with `torsocks`, which intercepts socket calls and covers programs that ignore proxy settings.
- `env`: `ALL_PROXY`, `HTTP_PROXY` and `HTTPS_PROXY` are set to a `socks5h://` URL. Only programs that honour these variables are routed; others connect directly and **can leak your real IP**.
- `auto`: `torsocks` if installed, otherwise `env` with a warning.

Use `--mode torsocks` when you need a guarantee.

## Development

```sh
make test   # go test -race -cover ./...
make lint   # golangci-lint
make vuln   # govulncheck
```

See [SECURITY.md](SECURITY.md) to report vulnerabilities.

## License

MIT. See [LICENSE](LICENSE).
