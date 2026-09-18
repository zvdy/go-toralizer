package tor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	// DefaultControlAddress is the address Tor listens on for control
	// connections when ControlPort is enabled.
	DefaultControlAddress = "127.0.0.1:9051"

	// DefaultControlTimeout bounds a control-port exchange.
	DefaultControlTimeout = 10 * time.Second

	// SignalNewNym asks Tor to build fresh circuits for new connections.
	SignalNewNym = "NEWNYM"
)

var (
	// ErrControl is returned when a control-port exchange fails.
	ErrControl = errors.New("tor control port error")

	// errRejected means Tor answered with a status other than 250.
	errRejected = errors.New("command rejected")
)

// ControlClient speaks the subset of the Tor control protocol needed to send
// signals, such as requesting a new circuit.
type ControlClient struct {
	// Address is the control port, as "host:port". Zero means
	// DefaultControlAddress.
	Address string

	// Password authenticates against a HashedControlPassword. Leave empty when
	// Tor is configured with no authentication.
	Password string

	// Timeout bounds the exchange. Zero means DefaultControlTimeout.
	Timeout time.Duration

	// dialContext is swapped out in tests.
	dialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

func (c *ControlClient) address() string {
	if c.Address != "" {
		return c.Address
	}
	return DefaultControlAddress
}

func (c *ControlClient) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultControlTimeout
}

func (c *ControlClient) dial(ctx context.Context, address string) (net.Conn, error) {
	if c.dialContext != nil {
		return c.dialContext(ctx, "tcp", address)
	}
	d := &net.Dialer{Timeout: c.timeout()}
	return d.DialContext(ctx, "tcp", address)
}

// Signal authenticates and sends a control signal, such as SignalNewNym.
func (c *ControlClient) Signal(ctx context.Context, signal string) error {
	signal = strings.ToUpper(strings.TrimSpace(signal))
	if signal == "" {
		return fmt.Errorf("%w: signal is empty", ErrControl)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	conn, err := c.dial(ctx, c.address())
	if err != nil {
		return fmt.Errorf("%w: connecting to %s: %w", ErrControl, c.address(), err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	reader := bufio.NewReader(conn)

	if err := c.exchange(conn, reader, authCommand(c.Password)); err != nil {
		return fmt.Errorf("%w: authenticating to %s: %w", ErrControl, c.address(), err)
	}
	if err := c.exchange(conn, reader, "SIGNAL "+signal); err != nil {
		return fmt.Errorf("%w: sending %s: %w", ErrControl, signal, err)
	}

	// Best effort: Tor closes the connection on QUIT, so a failure here says
	// nothing about whether the signal was accepted.
	_, _ = conn.Write([]byte("QUIT\r\n"))
	return nil
}

// NewNym asks Tor for a fresh circuit, which changes the exit address used by
// subsequent connections.
func (c *ControlClient) NewNym(ctx context.Context) error {
	return c.Signal(ctx, SignalNewNym)
}

func (c *ControlClient) exchange(conn net.Conn, reader *bufio.Reader, command string) error {
	if _, err := conn.Write([]byte(command + "\r\n")); err != nil {
		return fmt.Errorf("writing command: %w", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading reply: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "250") {
		return fmt.Errorf("%w: tor replied %q", errRejected, line)
	}
	return nil
}

// authCommand builds an AUTHENTICATE line. Tor's control protocol takes the
// password as a quoted string, so backslashes and quotes must be escaped.
func authCommand(password string) string {
	if password == "" {
		return "AUTHENTICATE"
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(password)
	return `AUTHENTICATE "` + escaped + `"`
}
