// Package tor checks that a SOCKS5 proxy is reachable and routes through Tor.
package tor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultPort      = "9050"
	DefaultVerifyURL = "https://check.torproject.org/api/ip"

	defaultDialTimeout = 5 * time.Second
	maxBodyBytes       = 64 << 10
)

// Checker validates a SOCKS proxy and confirms traffic exits through Tor.
type Checker interface {
	Reachable(ctx context.Context, proxy string) error
	Verify(ctx context.Context, proxy string) error
}

// Option configures a Checker.
type Option func(*checker)

// WithVerifyURL overrides the endpoint queried by Verify.
func WithVerifyURL(u string) Option { return func(c *checker) { c.verifyURL = u } }

// WithDialTimeout overrides the TCP dial and SOCKS handshake timeout.
func WithDialTimeout(d time.Duration) Option { return func(c *checker) { c.dialTimeout = d } }

type checker struct {
	verifyURL   string
	dialTimeout time.Duration
}

// NewChecker returns a Checker using the Tor Project check API.
func NewChecker(opts ...Option) Checker {
	c := &checker{verifyURL: DefaultVerifyURL, dialTimeout: defaultDialTimeout}
	for _, o := range opts {
		o(c)
	}
	return c
}

// NormalizeProxy accepts socks5 or socks5h proxies (or a bare host[:port]) and
// returns a socks5h://host:port URL. socks5h makes the proxy resolve hostnames,
// so DNS lookups go through Tor instead of leaking to the local resolver.
func NormalizeProxy(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("proxy address is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "socks5h://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid proxy %q: %w", raw, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h":
	default:
		return "", fmt.Errorf("unsupported proxy scheme %q: use socks5 or socks5h", u.Scheme)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("proxy %q has no host", raw)
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.User != nil {
		return "", fmt.Errorf("proxy %q must be of the form host[:port]", raw)
	}
	port := u.Port()
	if port == "" {
		port = DefaultPort
	}
	return "socks5h://" + net.JoinHostPort(u.Hostname(), port), nil
}

// Reachable dials the proxy and completes a SOCKS5 no-auth greeting.
func (c *checker) Reachable(ctx context.Context, proxy string) error {
	norm, err := NormalizeProxy(proxy)
	if err != nil {
		return err
	}
	u, _ := url.Parse(norm)

	d := net.Dialer{Timeout: c.dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return fmt.Errorf("dial %s: %w", u.Host, err)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(c.dialTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fmt.Errorf("socks5 greeting: %w", err)
	}
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return fmt.Errorf("socks5 greeting reply: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return fmt.Errorf("%s is not a usable SOCKS5 proxy (reply %#02x %#02x)", u.Host, resp[0], resp[1])
	}
	return nil
}

const verifyTimeout = 30 * time.Second

// Verify fetches the Tor check API through the proxy and requires IsTor.
func (c *checker) Verify(ctx context.Context, proxy string) error {
	norm, err := NormalizeProxy(proxy)
	if err != nil {
		return err
	}
	proxyURL, _ := url.Parse(norm)

	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			TLSHandshakeTimeout: 30 * time.Second,
			DisableKeepAlives:   true,
		},
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.verifyURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request via proxy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("check service returned %s", resp.Status)
	}
	var out struct {
		IsTor bool   `json:"IsTor"`
		IP    string `json:"IP"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&out); err != nil {
		return fmt.Errorf("decode check response: %w", err)
	}
	if !out.IsTor {
		return fmt.Errorf("traffic is not exiting through Tor (exit IP %s)", out.IP)
	}
	return nil
}
