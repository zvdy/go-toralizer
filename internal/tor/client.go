package tor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

const (
	// DefaultCheckURL reports whether the request arrived over Tor.
	DefaultCheckURL = "https://check.torproject.org/api/ip"

	// DefaultDialTimeout bounds the SOCKS reachability probe.
	DefaultDialTimeout = 5 * time.Second

	// DefaultCheckTimeout bounds the end-to-end connectivity check.
	DefaultCheckTimeout = 30 * time.Second

	// maxCheckBody caps how much of the check endpoint's response we read, so a
	// hostile or misconfigured proxy cannot make us allocate without bound.
	maxCheckBody = 64 << 10
)

var (
	// ErrProxyUnreachable means nothing usable is listening on the SOCKS port.
	ErrProxyUnreachable = errors.New("tor proxy is not reachable")

	// ErrNotTor means the proxy answered but traffic is not leaving over Tor.
	ErrNotTor = errors.New("traffic is not being routed through tor")

	// ErrCheckFailed means the connectivity check itself could not complete.
	ErrCheckFailed = errors.New("tor connectivity check failed")

	// errNoContextDialer means the SOCKS dialer cannot honour cancellation,
	// which would let a probe outlive its context.
	errNoContextDialer = errors.New("socks5 dialer does not support contexts")
)

// Status is the result of a successful connectivity check.
type Status struct {
	IsTor bool   `json:"IsTor"`
	IP    string `json:"IP"`
}

// Client probes and verifies a Tor SOCKS proxy.
type Client struct {
	// Proxy is the endpoint to probe. The zero value is not usable; build one
	// with ParseProxy.
	Proxy Proxy

	// DialTimeout bounds a single connection attempt. Zero means
	// DefaultDialTimeout.
	DialTimeout time.Duration

	// CheckTimeout bounds Verify. Zero means DefaultCheckTimeout.
	CheckTimeout time.Duration

	// CheckURL is the endpoint queried by Verify. Zero means DefaultCheckURL.
	CheckURL string

	// dialContext is swapped out in tests.
	dialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

// NewClient returns a Client for p with the default timeouts.
func NewClient(p Proxy) *Client {
	return &Client{Proxy: p}
}

func (c *Client) dialTimeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}
	return DefaultDialTimeout
}

func (c *Client) checkTimeout() time.Duration {
	if c.CheckTimeout > 0 {
		return c.CheckTimeout
	}
	return DefaultCheckTimeout
}

func (c *Client) checkURL() string {
	if c.CheckURL != "" {
		return c.CheckURL
	}
	return DefaultCheckURL
}

func (c *Client) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if c.dialContext != nil {
		return c.dialContext(ctx, network, address)
	}
	d := &net.Dialer{Timeout: c.dialTimeout()}
	return d.DialContext(ctx, network, address)
}

// Probe reports whether a SOCKS5 server is listening on the proxy address. It
// does more than a bare TCP connect: it performs the SOCKS5 greeting, so an
// unrelated service occupying the port is reported as unreachable rather than
// being mistaken for Tor.
func (c *Client) Probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.dialTimeout())
	defer cancel()

	conn, err := c.dial(ctx, "tcp", c.Proxy.Address())
	if err != nil {
		return fmt.Errorf("%w on %s: %w", ErrProxyUnreachable, c.Proxy.Address(), err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// Version 5, one method offered: 0x00 (no authentication) when the proxy
	// needs no credentials, 0x02 (username/password) otherwise.
	method := byte(0x00)
	if c.Proxy.Auth() != nil {
		method = 0x02
	}
	if _, err := conn.Write([]byte{0x05, 0x01, method}); err != nil {
		return fmt.Errorf("%w on %s: %w", ErrProxyUnreachable, c.Proxy.Address(), err)
	}

	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("%w on %s: %w", ErrProxyUnreachable, c.Proxy.Address(), err)
	}
	if reply[0] != 0x05 {
		return fmt.Errorf("%w on %s: not a socks5 server (got version 0x%02x)",
			ErrProxyUnreachable, c.Proxy.Address(), reply[0])
	}
	if reply[1] == 0xff {
		return fmt.Errorf("%w on %s: proxy rejected all offered auth methods",
			ErrProxyUnreachable, c.Proxy.Address())
	}
	return nil
}

// Dialer returns a dialer that routes connections through the proxy.
func (c *Client) Dialer() (proxy.ContextDialer, error) {
	forward := &net.Dialer{Timeout: c.dialTimeout()}
	d, err := proxy.SOCKS5("tcp", c.Proxy.Address(), c.Proxy.Auth(), forward)
	if err != nil {
		return nil, fmt.Errorf("build socks5 dialer for %s: %w", c.Proxy.Address(), err)
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errNoContextDialer, c.Proxy.Address())
	}
	return cd, nil
}

// HTTPClient returns an *http.Client whose transport dials through the proxy.
func (c *Client) HTTPClient() (*http.Client, error) {
	d, err := c.Dialer()
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: c.checkTimeout(),
		Transport: &http.Transport{
			DialContext:           d.DialContext,
			Proxy:                 nil, // never fall back to the ambient proxy env
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   c.checkTimeout(),
			ExpectContinueTimeout: time.Second,
		},
	}, nil
}

// Verify performs a request through the proxy and reports what the far end
// saw. It returns ErrNotTor when the request completed but did not exit the
// Tor network, and ErrCheckFailed when the check could not be completed.
func (c *Client) Verify(ctx context.Context) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, c.checkTimeout())
	defer cancel()

	httpClient, err := c.HTTPClient()
	if err != nil {
		return Status{}, fmt.Errorf("%w: %w", ErrCheckFailed, err)
	}
	defer httpClient.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.checkURL(), nil)
	if err != nil {
		return Status{}, fmt.Errorf("%w: %w", ErrCheckFailed, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("%w: request to %s: %w", ErrCheckFailed, c.checkURL(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("%w: %s returned %s", ErrCheckFailed, c.checkURL(), resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCheckBody))
	if err != nil {
		return Status{}, fmt.Errorf("%w: reading response: %w", ErrCheckFailed, err)
	}

	var status Status
	if err := json.Unmarshal(body, &status); err != nil {
		return Status{}, fmt.Errorf("%w: decoding response from %s: %w", ErrCheckFailed, c.checkURL(), err)
	}
	if !status.IsTor {
		return status, fmt.Errorf("%w: exit address %s is not a tor relay", ErrNotTor, status.IP)
	}
	return status, nil
}
