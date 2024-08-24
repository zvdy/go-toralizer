package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
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
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Enable verbose output",
					},
					&cli.DurationFlag{
						Name:    "timeout",
						Aliases: []string{"t"},
						Usage:   "Specify a timeout for the command execution",
						Value:   30 * time.Second,
					},
					&cli.StringFlag{
						Name:    "proxy",
						Aliases: []string{"p"},
						Usage:   "Specify a custom proxy address",
						Value:   "socks5://127.0.0.1:9050",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Redirect command output to a file",
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
	verbose := c.Bool("verbose")
	timeout := c.Duration("timeout")
	proxy := c.String("proxy")
	outputFile := c.String("output")

	// Check if Tor proxy is running
	if !isTorProxyRunning(proxy) {
		return fmt.Errorf("tor proxy is not running on %s", proxy)
	}

	// Set the proxy environment variables
	os.Setenv("HTTP_PROXY", proxy)
	os.Setenv("HTTPS_PROXY", proxy)
	os.Setenv("ALL_PROXY", proxy)

	// Verify the IP address
	if err := verifyTorConnection(proxy); err != nil {
		return err
	}

	// Execute the command
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if outputFile != "" {
		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %v", err)
		}
		defer file.Close()
		cmd.Stdout = file
		cmd.Stderr = file
	}

	if verbose {
		fmt.Printf("Executing command: %s\n", command)
	}

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return fmt.Errorf("command timed out")
	case err := <-done:
		if err != nil {
			return fmt.Errorf("command failed: %v", err)
		}
	}

	return nil
}

func isTorProxyRunning(proxy string) bool {
	// Parse the proxy URL to extract the host and port
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		fmt.Printf("Invalid proxy URL: %v\n", err)
		return false
	}

	hostPort := proxyURL.Host
	if !strings.Contains(hostPort, ":") {
		hostPort += ":9050" // Default port for Tor SOCKS5 proxy
	}

	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", hostPort, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	fmt.Print("Tor proxy is running\n")
	return true
}

func verifyTorConnection(proxy string) error {
	cmd := exec.Command("curl", "--socks5", proxy, "https://check.torproject.org/api/ip")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to verify Tor connection: %v", err)
	}

	fmt.Printf("Tor verification output: %s\n", output)
	return nil
}
