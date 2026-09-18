package tor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Proxy
		wantErr string
	}{
		{
			name:  "full socks5 url",
			input: "socks5://127.0.0.1:9050",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9050},
		},
		{
			name:  "socks5h keeps dns remote",
			input: "socks5h://127.0.0.1:9150",
			want:  Proxy{Scheme: SchemeSOCKS5H, Host: "127.0.0.1", Port: 9150},
		},
		{
			name:  "bare host and port",
			input: "127.0.0.1:9150",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9150},
		},
		{
			name:  "host without port uses the default",
			input: "socks5://localhost",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "localhost", Port: DefaultSOCKSPort},
		},
		{
			name:  "ipv6 literal",
			input: "socks5://[::1]:9050",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "::1", Port: 9050},
		},
		{
			name:  "scheme is case insensitive",
			input: "SOCKS5://127.0.0.1:9050",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9050},
		},
		{
			name:  "socks is an alias for socks5",
			input: "socks://127.0.0.1:9050",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9050},
		},
		{
			name:  "credentials are carried through",
			input: "socks5://user:pass@127.0.0.1:9050",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9050, Username: "user", Password: "pass"},
		},
		{
			name:  "surrounding whitespace is ignored",
			input: "  socks5://127.0.0.1:9050  ",
			want:  Proxy{Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 9050},
		},
		{name: "empty", input: "", wantErr: "address is empty"},
		{name: "whitespace only", input: "   ", wantErr: "address is empty"},
		{name: "http is not a socks proxy", input: "http://127.0.0.1:8080", wantErr: "unsupported scheme"},
		{name: "missing host", input: "socks5://:9050", wantErr: "missing host"},
		{name: "port out of range", input: "socks5://127.0.0.1:70000", wantErr: "invalid port"},
		{name: "non-numeric port", input: "socks5://127.0.0.1:abc", wantErr: "invalid port"},
		{name: "unexpected path", input: "socks5://127.0.0.1:9050/tor", wantErr: "unexpected path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseProxy(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidProxy)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProxyAccessors(t *testing.T) {
	t.Parallel()

	p, err := ParseProxy("socks5://127.0.0.1:9050")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9050", p.Address())
	assert.Equal(t, "socks5://127.0.0.1:9050", p.String())
	assert.Nil(t, p.Auth())

	v6, err := ParseProxy("socks5://[::1]:9050")
	require.NoError(t, err)
	assert.Equal(t, "[::1]:9050", v6.Address())

	withAuth, err := ParseProxy("socks5://user:pass@127.0.0.1:9050")
	require.NoError(t, err)
	require.NotNil(t, withAuth.Auth())
	assert.Equal(t, "user", withAuth.Auth().User)
	assert.Equal(t, "pass", withAuth.Auth().Password)
}

func TestProxyEnvURLAlwaysResolvesRemotely(t *testing.T) {
	t.Parallel()

	// socks5 asks the client to resolve names locally, which would leak the
	// hostnames being visited to the local resolver. EnvURL must upgrade it.
	p, err := ParseProxy("socks5://127.0.0.1:9050")
	require.NoError(t, err)
	assert.Equal(t, "socks5h://127.0.0.1:9050", p.EnvURL())

	withAuth, err := ParseProxy("socks5://user:pass@127.0.0.1:9050")
	require.NoError(t, err)
	assert.Equal(t, "socks5h://user:pass@127.0.0.1:9050", withAuth.EnvURL())
}
