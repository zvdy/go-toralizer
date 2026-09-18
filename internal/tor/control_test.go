package tor

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// controlServer is a stand-in for Tor's control port.
type controlServer struct {
	listener net.Listener
	wg       sync.WaitGroup

	// replies maps a command verb to the status line to answer with. A missing
	// verb is answered with "250 OK".
	replies map[string]string

	mu       sync.Mutex
	received []string
}

func startControlServer(t *testing.T, replies map[string]string) *controlServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &controlServer{listener: listener, replies: replies}

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

func (s *controlServer) Addr() string { return s.listener.Addr().String() }

func (s *controlServer) Received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.received...)
}

func (s *controlServer) handle(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		s.mu.Lock()
		s.received = append(s.received, line)
		s.mu.Unlock()

		verb, _, _ := strings.Cut(line, " ")
		if verb == "QUIT" {
			_, _ = conn.Write([]byte("250 closing connection\r\n"))
			return
		}

		reply, ok := s.replies[verb]
		if !ok {
			reply = "250 OK"
		}
		if _, err := conn.Write([]byte(reply + "\r\n")); err != nil {
			return
		}
	}
}

func TestControlClientNewNym(t *testing.T) {
	t.Parallel()

	srv := startControlServer(t, nil)
	client := &ControlClient{Address: srv.Addr(), Timeout: 5 * time.Second}

	require.NoError(t, client.NewNym(t.Context()))

	received := srv.Received()
	require.GreaterOrEqual(t, len(received), 2)
	assert.Equal(t, "AUTHENTICATE", received[0])
	assert.Equal(t, "SIGNAL NEWNYM", received[1])
}

func TestControlClientSendsQuotedPassword(t *testing.T) {
	t.Parallel()

	srv := startControlServer(t, nil)
	client := &ControlClient{
		Address:  srv.Addr(),
		Password: `he said "hi" \ bye`,
		Timeout:  5 * time.Second,
	}

	require.NoError(t, client.NewNym(t.Context()))
	assert.Equal(t, `AUTHENTICATE "he said \"hi\" \\ bye"`, srv.Received()[0])
}

func TestControlClientRejectsBadAuth(t *testing.T) {
	t.Parallel()

	srv := startControlServer(t, map[string]string{
		"AUTHENTICATE": "515 Authentication failed: Password did not match",
	})
	client := &ControlClient{Address: srv.Addr(), Password: "wrong", Timeout: 5 * time.Second}

	err := client.NewNym(t.Context())
	require.ErrorIs(t, err, ErrControl)
	assert.Contains(t, err.Error(), "authenticating")
	assert.Contains(t, err.Error(), "515")
	assert.NotContains(t, srv.Received(), "SIGNAL NEWNYM")
}

func TestControlClientReportsRejectedSignal(t *testing.T) {
	t.Parallel()

	srv := startControlServer(t, map[string]string{"SIGNAL": "552 Unrecognized signal"})
	client := &ControlClient{Address: srv.Addr(), Timeout: 5 * time.Second}

	err := client.Signal(t.Context(), "bogus")
	require.ErrorIs(t, err, ErrControl)
	assert.Contains(t, err.Error(), "sending BOGUS")
}

func TestControlClientRejectsEmptySignal(t *testing.T) {
	t.Parallel()

	client := &ControlClient{Address: "127.0.0.1:1"}
	err := client.Signal(t.Context(), "   ")
	require.ErrorIs(t, err, ErrControl)
	assert.Contains(t, err.Error(), "signal is empty")
}

func TestControlClientReportsUnreachablePort(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	client := &ControlClient{Address: addr, Timeout: 2 * time.Second}
	err = client.NewNym(t.Context())
	require.ErrorIs(t, err, ErrControl)
	assert.Contains(t, err.Error(), "connecting to")
}

func TestControlClientDefaults(t *testing.T) {
	t.Parallel()

	client := &ControlClient{}
	assert.Equal(t, DefaultControlAddress, client.address())
	assert.Equal(t, DefaultControlTimeout, client.timeout())
}
