package main

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
)

func TestExecCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		isTorRunning   bool
		verifyTorError error
		expectedError  string
	}{
		{
			name:          "Tor proxy not running",
			args:          []string{"go-toralizer", "exec", "--command", "echo test"},
			isTorRunning:  false,
			expectedError: "tor proxy is not running on socks5://127.0.0.1:9050",
		},
		{
			name:           "Tor verification failed",
			args:           []string{"go-toralizer", "exec", "--command", "echo test"},
			isTorRunning:   true,
			verifyTorError: errors.New("failed to verify Tor connection"),
			expectedError:  "failed to verify Tor connection",
		},
		{
			name:          "Command execution timeout",
			args:          []string{"go-toralizer", "exec", "--command", "sleep 2", "--timeout", "1s"},
			isTorRunning:  true,
			expectedError: "command execution failed: signal: killed",
		},
		{
			name:          "Successful command execution",
			args:          []string{"go-toralizer", "exec", "--command", "echo test"},
			isTorRunning:  true,
			expectedError: "",
		},
		{
			name:          "Invalid command",
			args:          []string{"go-toralizer", "exec", "--command", "invalidcommand"},
			isTorRunning:  true,
			expectedError: "command execution failed",
		},
		{
			name:          "Missing command argument",
			args:          []string{"go-toralizer", "exec"},
			isTorRunning:  true,
			expectedError: "Required flag \"command\" not set",
		},
		{
			name:          "Invalid timeout format",
			args:          []string{"go-toralizer", "exec", "--command", "echo test", "--timeout", "invalid"},
			isTorRunning:  true,
			expectedError: "invalid value \"invalid\" for flag -timeout: parse error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock isTorProxyRunning
			mockIsTorProxyRunning := func(proxy string) bool {
				return tt.isTorRunning
			}

			// Mock verifyTorConnection
			mockVerifyTorConnection := func(proxy string) error {
				return tt.verifyTorError
			}

			// Capture stdout
			r, w, _ := os.Pipe()
			stdout := os.Stdout
			os.Stdout = w

			// Create a new CLI app
			app := &cli.App{
				Commands: []*cli.Command{
					{
						Name: "exec",
						Action: func(c *cli.Context) error {
							return execCommand(c, mockIsTorProxyRunning, mockVerifyTorConnection)
						},
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "command",
								Aliases:  []string{"c"},
								Required: true,
							},
							&cli.DurationFlag{
								Name:    "timeout",
								Aliases: []string{"t"},
								Value:   30 * time.Second,
							},
							&cli.StringFlag{
								Name:    "proxy",
								Aliases: []string{"p"},
								Value:   "socks5://127.0.0.1:9050",
							},
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
							},
						},
					},
				},
			}

			// Run the app with test arguments
			err := app.Run(tt.args)

			// Close the writer and restore stdout
			w.Close()
			os.Stdout = stdout

			// Read captured output
			var buf bytes.Buffer
			buf.ReadFrom(r)

			// Check for expected error
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
