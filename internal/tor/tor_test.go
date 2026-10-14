package tor

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNormalizeProxy(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{"127.0.0.1", "socks5h://127.0.0.1:9050", ""},
		{"127.0.0.1:9150", "socks5h://127.0.0.1:9150", ""},
		{"socks5://localhost", "socks5h://localhost:9050", ""},
		{"socks5h://localhost:1080", "socks5h://localhost:1080", ""},
		{"SOCKS5://example.com:1", "socks5h://example.com:1", ""},
		{"[::1]:9050", "socks5h://[::1]:9050", ""},
		{"  socks5://h:1  ", "socks5h://h:1", ""},
		{"http://127.0.0.1:8080", "", "unsupported proxy scheme"},
		{"socks4://127.0.0.1", "", "unsupported proxy scheme"},
		{"", "", "empty"},
		{"socks5://", "", "no host"},
		{"socks5://u:p@host:1", "", "host[:port]"},
		{"socks5://host:1/path", "", "host[:port]"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := NormalizeProxy(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func fakeSOCKS(t *testing.T, reply []byte) string {
	t.Helper()
	ln := listen(t)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				buf := make([]byte, 3)
				if _, err := conn.Read(buf); err != nil {
					return
				}
				_, _ = conn.Write(reply)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestReachable(t *testing.T) {
	c := NewChecker(WithDialTimeout(time.Second))
	ctx := context.Background()

	if err := c.Reachable(ctx, fakeSOCKS(t, []byte{5, 0})); err != nil {
		t.Fatalf("valid socks server: %v", err)
	}
	if err := c.Reachable(ctx, fakeSOCKS(t, []byte{5, 0xff})); err == nil {
		t.Fatal("expected error for rejected auth method")
	}
	if err := c.Reachable(ctx, fakeSOCKS(t, []byte("HT"))); err == nil {
		t.Fatal("expected error for non-socks server")
	}

	ln := listen(t)
	addr := ln.Addr().String()
	_ = ln.Close()
	if err := c.Reachable(ctx, addr); err == nil {
		t.Fatal("expected dial error")
	}

	if err := c.Reachable(ctx, "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected scheme error")
	}
}

// tunnelSOCKS is a minimal SOCKS5 server that accepts CONNECT and pipes to the
// requested target, recording the address type used.
func tunnelSOCKS(t *testing.T, atyps chan<- byte) string {
	t.Helper()
	ln := listen(t)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				hdr := make([]byte, 3)
				if _, err := io.ReadFull(conn, hdr); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0})
				req := make([]byte, 4)
				if _, err := io.ReadFull(conn, req); err != nil {
					return
				}
				var host string
				switch req[3] {
				case 1:
					ip := make([]byte, 4)
					_, _ = io.ReadFull(conn, ip)
					host = net.IP(ip).String()
				case 3:
					l := make([]byte, 1)
					_, _ = io.ReadFull(conn, l)
					name := make([]byte, l[0])
					_, _ = io.ReadFull(conn, name)
					host = string(name)
				default:
					return
				}
				port := make([]byte, 2)
				_, _ = io.ReadFull(conn, port)
				atyps <- req[3]
				var d net.Dialer
				target, err := d.DialContext(context.Background(), "tcp", net.JoinHostPort(host, strconv.Itoa(int(port[0])<<8|int(port[1]))))
				if err != nil {
					return
				}
				defer func() { _ = target.Close() }()
				_, _ = conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go func() { _, _ = io.Copy(target, conn) }()
				_, _ = io.Copy(conn, target)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestVerify(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"tor", 200, `{"IsTor":true,"IP":"1.2.3.4"}`, ""},
		{"not tor", 200, `{"IsTor":false,"IP":"1.2.3.4"}`, "not exiting through Tor"},
		{"bad json", 200, `nope`, "decode"},
		{"bad status", 500, `{"IsTor":true}`, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			atyps := make(chan byte, 4)
			proxy := tunnelSOCKS(t, atyps)
			c := NewChecker(WithVerifyURL(srv.URL), WithDialTimeout(time.Second))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := c.Verify(ctx, proxy)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
			select {
			case <-atyps:
			default:
				t.Fatal("request did not go through the SOCKS proxy")
			}
		})
	}
}

func TestVerifyInvalidProxy(t *testing.T) {
	if err := NewChecker().Verify(context.Background(), "http://x:1"); err == nil {
		t.Fatal("expected error")
	}
}
