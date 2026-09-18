package tor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientProbe(t *testing.T) {
	t.Parallel()

	t.Run("accepts a socks5 server", func(t *testing.T) {
		t.Parallel()

		srv := startSOCKSServer(t, &socksServer{})
		client := NewClient(srv.Proxy(t))
		assert.NoError(t, client.Probe(t.Context()))
	})

	t.Run("rejects a port with nothing listening", func(t *testing.T) {
		t.Parallel()

		// Bind then close, so the port is almost certainly free.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())

		p, err := ParseProxy("socks5://" + addr)
		require.NoError(t, err)
		err = NewClient(p).Probe(t.Context())
		require.ErrorIs(t, err, ErrProxyUnreachable)
	})

	t.Run("rejects a non-socks service on the port", func(t *testing.T) {
		t.Parallel()

		srv := startSOCKSServer(t, &socksServer{garbageGreeting: true})
		err := NewClient(srv.Proxy(t)).Probe(t.Context())
		require.ErrorIs(t, err, ErrProxyUnreachable)
		assert.Contains(t, err.Error(), "not a socks5 server")
	})

	t.Run("reports rejected authentication", func(t *testing.T) {
		t.Parallel()

		srv := startSOCKSServer(t, &socksServer{rejectAuth: true})
		err := NewClient(srv.Proxy(t)).Probe(t.Context())
		require.ErrorIs(t, err, ErrProxyUnreachable)
		assert.Contains(t, err.Error(), "rejected all offered auth methods")
	})

	t.Run("honours the dial timeout", func(t *testing.T) {
		t.Parallel()

		p, err := ParseProxy("socks5://127.0.0.1:9050")
		require.NoError(t, err)
		client := &Client{
			Proxy:       p,
			DialTimeout: 50 * time.Millisecond,
			dialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
		err = client.Probe(t.Context())
		require.ErrorIs(t, err, ErrProxyUnreachable)
	})
}

func TestClientVerify(t *testing.T) {
	t.Parallel()

	newClient := func(t *testing.T, handler http.Handler) *Client {
		t.Helper()
		backend := httptest.NewServer(handler)
		t.Cleanup(backend.Close)

		srv := startSOCKSServer(t, &socksServer{})
		client := NewClient(srv.Proxy(t))
		client.CheckURL = backend.URL
		client.CheckTimeout = 10 * time.Second
		return client
	}

	t.Run("reports the exit address when traffic is over tor", func(t *testing.T) {
		t.Parallel()

		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"IsTor":true,"IP":"185.220.101.1"}`))
		}))

		status, err := client.Verify(t.Context())
		require.NoError(t, err)
		assert.True(t, status.IsTor)
		assert.Equal(t, "185.220.101.1", status.IP)
	})

	t.Run("reports traffic that is not leaving over tor", func(t *testing.T) {
		t.Parallel()

		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"IsTor":false,"IP":"203.0.113.7"}`))
		}))

		status, err := client.Verify(t.Context())
		require.ErrorIs(t, err, ErrNotTor)
		assert.Equal(t, "203.0.113.7", status.IP)
		assert.Contains(t, err.Error(), "203.0.113.7")
	})

	t.Run("reports a non-200 response", func(t *testing.T) {
		t.Parallel()

		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusServiceUnavailable)
		}))

		_, err := client.Verify(t.Context())
		require.ErrorIs(t, err, ErrCheckFailed)
		assert.Contains(t, err.Error(), "503")
	})

	t.Run("reports an undecodable response", func(t *testing.T) {
		t.Parallel()

		client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))

		_, err := client.Verify(t.Context())
		require.ErrorIs(t, err, ErrCheckFailed)
	})

	t.Run("reports an unreachable proxy", func(t *testing.T) {
		t.Parallel()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())

		p, err := ParseProxy("socks5://" + addr)
		require.NoError(t, err)
		client := NewClient(p)
		client.CheckURL = "http://example.invalid/"

		_, err = client.Verify(t.Context())
		require.ErrorIs(t, err, ErrCheckFailed)
	})
}

func TestClientHTTPClientIgnoresAmbientProxyEnv(t *testing.T) {
	// Not parallel: it sets process-wide environment.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	srv := startSOCKSServer(t, &socksServer{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"IsTor":true,"IP":"185.220.101.2"}`))
	}))
	t.Cleanup(backend.Close)

	client := NewClient(srv.Proxy(t))
	client.CheckURL = backend.URL

	status, err := client.Verify(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "185.220.101.2", status.IP)
}

func TestClientDefaults(t *testing.T) {
	t.Parallel()

	client := NewClient(Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9050})
	assert.Equal(t, DefaultDialTimeout, client.dialTimeout())
	assert.Equal(t, DefaultCheckTimeout, client.checkTimeout())
	assert.Equal(t, DefaultCheckURL, client.checkURL())

	client.DialTimeout = time.Second
	client.CheckTimeout = 2 * time.Second
	client.CheckURL = "http://example.test/"
	assert.Equal(t, time.Second, client.dialTimeout())
	assert.Equal(t, 2*time.Second, client.checkTimeout())
	assert.Equal(t, "http://example.test/", client.checkURL())
}
