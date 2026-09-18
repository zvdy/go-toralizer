package tor

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// socksServer is a minimal SOCKS5 server used to exercise the client without a
// real Tor daemon. It supports the no-auth method and the CONNECT command.
type socksServer struct {
	listener net.Listener
	wg       sync.WaitGroup

	// rejectAuth makes the server answer the greeting with 0xff, as Tor does
	// when the client offers no acceptable authentication method.
	rejectAuth bool

	// garbageGreeting makes the server answer with something that is not
	// SOCKS5, standing in for an unrelated service on the port.
	garbageGreeting bool
}

// startSOCKSServer starts the server and registers cleanup with t.
func startSOCKSServer(t *testing.T, srv *socksServer) *socksServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv.listener = listener

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			srv.wg.Add(1)
			go func() {
				defer srv.wg.Done()
				defer func() { _ = conn.Close() }()
				srv.handle(conn)
			}()
		}
	}()

	t.Cleanup(func() {
		_ = listener.Close()
		srv.wg.Wait()
	})
	return srv
}

// Addr returns the "host:port" the server is listening on.
func (s *socksServer) Addr() string { return s.listener.Addr().String() }

// Proxy returns a parsed Proxy pointing at the server.
func (s *socksServer) Proxy(t *testing.T) Proxy {
	t.Helper()
	p, err := ParseProxy("socks5://" + s.Addr())
	require.NoError(t, err)
	return p
}

func (s *socksServer) handle(conn net.Conn) {
	if s.garbageGreeting {
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if s.rejectAuth {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	target, err := readConnectRequest(conn)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = upstream.Close() }()

	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	// Once either direction ends, tear both down. Leaving the upstream half
	// open would keep the test's HTTP server from shutting down.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, conn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, upstream); done <- struct{}{} }()

	<-done
	_ = conn.Close()
	_ = upstream.Close()
	<-done
}

// readConnectRequest parses a SOCKS5 CONNECT request and returns its target.
func readConnectRequest(conn net.Conn) (string, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return "", err
	}
	if head[0] != 0x05 || head[1] != 0x01 {
		return "", errors.New("unsupported socks request")
	}

	var host string
	switch head[3] {
	case 0x01: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	case 0x03: // domain name
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", err
		}
		name := make([]byte, length[0])
		if _, err := io.ReadFull(conn, name); err != nil {
			return "", err
		}
		host = string(name)
	case 0x04: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	default:
		return "", errors.New("unsupported address type")
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))), nil
}
