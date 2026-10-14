# Security Policy

## Supported Versions

Security fixes are provided for the latest released version and the `main` branch.

## Reporting a Vulnerability

Please report vulnerabilities privately through GitHub security advisories:
<https://github.com/zvdy/go-toralizer/security/advisories/new>

Do not open public issues for security problems. Expect an initial response
within a few days. Please include affected versions, reproduction steps and
impact.

## Scope

go-toralizer routes traffic through a SOCKS proxy (such as Tor) for programs
that honour proxy settings. It does not guarantee anonymity: programs that are
not proxy-aware may bypass the proxy and leak DNS queries or connections, and
the tool does not protect against application-level identifying data. Reports
about such leaks caused by the target program's behaviour are out of scope.
