package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "go-toralizer",
		Usage: "A CLI tool to run commands through Tor",
		Commands: []*cli.Command{
			{
				Name:   "exec",
				Usage:  "Execute a command through Tor",
				Action: execCommand,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "command",
						Aliases:  []string{"c"},
						Usage:    "Command to execute",
						Required: true,
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func execCommand(c *cli.Context) error {
	command := c.String("command")

	// Check if Tor proxy is running
	if !isTorProxyRunning() {
		return fmt.Errorf("Tor proxy is not running on 127.0.0.1:9050")
	}

	// Set the proxy environment variables
	os.Setenv("HTTP_PROXY", "socks5://127.0.0.1:9050")
	os.Setenv("HTTPS_PROXY", "socks5://127.0.0.1:9050")
	os.Setenv("ALL_PROXY", "socks5://127.0.0.1:9050")

	// Verify the IP address
	if err := verifyTorConnection(); err != nil {
		return err
	}

	// Execute the command
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to execute command: %v", err)
	}

	return nil
}

func isTorProxyRunning() bool {
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", "127.0.0.1:9050", timeout)
	if err != nil {
		return false
	}
	conn.Close()
	fmt.Print("Tor proxy is running\n")
	return true
}

func verifyTorConnection() error {
	cmd := exec.Command("curl", "--socks5", "127.0.0.1:9050", "https://check.torproject.org/api/ip")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to verify Tor connection: %v", err)
	}

	fmt.Printf("Tor verification output: %s\n", output)
	return nil
}
