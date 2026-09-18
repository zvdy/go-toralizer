package app

// Exit codes. Anything the child process returns is passed through unchanged,
// so go-toralizer's own failures use codes in the range shells reserve for
// wrapper tools to keep the two distinguishable.
const (
	// ExitOK means the command ran and exited zero.
	ExitOK = 0

	// ExitFailure is a generic go-toralizer failure.
	ExitFailure = 1

	// ExitUsage means the arguments were rejected.
	ExitUsage = 2

	// ExitProxyUnreachable means no Tor SOCKS proxy answered.
	ExitProxyUnreachable = 64

	// ExitNotTor means the proxy answered but traffic is not leaving over Tor.
	ExitNotTor = 65

	// ExitTimeout means the command exceeded its timeout.
	ExitTimeout = 66

	// ExitControl means a Tor control-port operation failed.
	ExitControl = 67
)
