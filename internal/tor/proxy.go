// Package tor provides the pieces needed to talk to a local Tor daemon: a
// parser for SOCKS proxy addresses, a reachability probe and connectivity
// check for the SOCKS port, and a minimal client for the control port.
package tor

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/proxy"
)

const (
	// DefaultSOCKSAddress is the address Tor listens on for SOCKS by default.
	DefaultSOCKSAddress = "socks5://127.0.0.1:9050"

	// DefaultSOCKSPort is used when an address is given without a port.
	DefaultSOCKSPort = 9050

	// SchemeSOCKS5 resolves hostnames locally before connecting.
	SchemeSOCKS5 = "socks5"

	// SchemeSOCKS5H resolves hostnames on the proxy, which keeps DNS lookups
	// inside the Tor network.
	SchemeSOCKS5H = "socks5h"
)

// ErrInvalidProxy is returned by ParseProxy for addresses it cannot use.
var ErrInvalidProxy = errors.New("invalid tor proxy address")

// Proxy is a validated Tor SOCKS proxy endpoint.
type Proxy struct {
	Scheme   string
	Host     string
	Port     int
	Username string
	Password string
}

// ParseProxy accepts "socks5://host:port", "socks5h://host:port" or a bare
// "host:port" and returns the endpoint it describes. The port defaults to
// DefaultSOCKSPort and the scheme to SchemeSOCKS5.
func ParseProxy(raw string) (Proxy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Proxy{}, fmt.Errorf("%w: address is empty", ErrInvalidProxy)
	}

	// A bare "host:port" has no scheme; url.Parse would read "host" as one.
	if !strings.Contains(raw, "//") {
		raw = SchemeSOCKS5 + "://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Proxy{}, fmt.Errorf("%w: %q: %w", ErrInvalidProxy, raw, err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case SchemeSOCKS5, SchemeSOCKS5H:
	case "socks":
		scheme = SchemeSOCKS5
	default:
		return Proxy{}, fmt.Errorf(
			"%w: unsupported scheme %q (want %s or %s)",
			ErrInvalidProxy, u.Scheme, SchemeSOCKS5, SchemeSOCKS5H,
		)
	}

	if u.Path != "" && u.Path != "/" {
		return Proxy{}, fmt.Errorf("%w: unexpected path %q", ErrInvalidProxy, u.Path)
	}

	host := u.Hostname()
	if host == "" {
		return Proxy{}, fmt.Errorf("%w: missing host in %q", ErrInvalidProxy, raw)
	}

	port := DefaultSOCKSPort
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return Proxy{}, fmt.Errorf("%w: invalid port %q", ErrInvalidProxy, p)
		}
	}

	p := Proxy{Scheme: scheme, Host: host, Port: port}
	if u.User != nil {
		p.Username = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	return p, nil
}

// Address returns the "host:port" form used when dialling the proxy.
func (p Proxy) Address() string {
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

// String returns the canonical URL form, without any credentials.
func (p Proxy) String() string {
	return p.Scheme + "://" + p.Address()
}

// EnvURL returns the URL to hand to child processes. It always uses socks5h so
// that DNS resolution happens inside Tor rather than on the local resolver,
// which would leak the hostnames being visited.
func (p Proxy) EnvURL() string {
	u := url.URL{Scheme: SchemeSOCKS5H, Host: p.Address()}
	if p.Username != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}

// Auth returns SOCKS credentials, or nil when the proxy needs none.
func (p Proxy) Auth() *proxy.Auth {
	if p.Username == "" && p.Password == "" {
		return nil
	}
	return &proxy.Auth{User: p.Username, Password: p.Password}
}
