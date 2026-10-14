<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img src="assets/logo.png" alt="go-toralizer" width="420">
  </picture>
</p>

<p align="center">
  Run any command through Tor.
</p>

<p align="center">
  <a href="https://github.com/zvdy/go-toralizer/actions/workflows/ci.yml"><img src="https://github.com/zvdy/go-toralizer/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/zvdy/go-toralizer"><img src="https://pkg.go.dev/badge/github.com/zvdy/go-toralizer.svg" alt="Go Reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT License"></a>
</p>

---

`go-toralizer` checks that your Tor proxy is alive and exits through Tor, then runs a command with its traffic routed through it.

```sh
go-toralizer exec -- curl -s https://check.torproject.org/api/ip
```

## Install

```sh
go install github.com/zvdy/go-toralizer/cmd/go-toralizer@latest
```

Or build from source with `make build`. Requires a running Tor daemon (`127.0.0.1:9050`, or `9150` for Tor Browser).

## Usage

```sh
# argv form, no shell involved
go-toralizer exec -- curl https://example.com

# shell form
go-toralizer exec -c "curl -s https://example.com | head"

# custom proxy, timeout and output file
go-toralizer exec -p 127.0.0.1:9150 -t 60s -o out.txt -- curl https://example.com
```

| Flag | Default | |
| --- | --- | --- |
| `-c, --command` | | Shell command line, as an alternative to argv after `--` |
| `-p, --proxy` | `socks5h://127.0.0.1:9050` | Tor SOCKS5 proxy |
| `-t, --timeout` | `30s` | Kill the command after this long (`0` disables); exits `124` |
| `-o, --output` | | Also write output to a file (`0600`) |
| `--mode` | `auto` | `auto`, `env` or `torsocks` |
| `--skip-verify` | | Skip the exit-node check |
| `-v, --verbose` | | Log execution details to stderr |

The command's exit code is passed through.

## How it routes

| Mode | Behaviour |
| --- | --- |
| `torsocks` | Wraps the command with `torsocks`, covering programs that ignore proxy settings. |
| `env` | Sets `ALL_PROXY`, `HTTP_PROXY` and `HTTPS_PROXY` to a `socks5h://` URL. |
| `auto` | `torsocks` if installed, otherwise `env` with a warning. |

> [!WARNING]
> In `env` mode only programs that honour proxy variables are routed. Others connect directly and can leak your real IP. Use `--mode torsocks` when you need a guarantee.

## Development

```sh
make test   # go test -race -cover ./...
make lint   # golangci-lint
make vuln   # govulncheck
```

Report vulnerabilities as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
