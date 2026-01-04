package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/urfave/cli/v2"
)

var (
	isTorProxyRunning   = defaultIsTorProxyRunning
	verifyTorConnection = defaultVerifyTorConnection
)

func defaultIsTorProxyRunning(proxy string) bool {
	// Default implementation
	return true
}

func defaultVerifyTorConnection(proxy string) error {
	// Default implementation
	return nil
}

func execCommand(c *cli.Context, isTorProxyRunning func(string) bool, verifyTorConnection func(string) error) error {
	command := c.String("command")
	timeout := c.Duration("timeout")
	proxy := c.String("proxy")
	outputFile := c.String("output")

	if !isTorProxyRunning(proxy) {
		return fmt.Errorf("tor proxy is not running on %s", proxy)
	}

	if err := verifyTorConnection(proxy); err != nil {
		return err
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = os.Environ()

	if outputFile != "" {
		output, err := cmd.CombinedOutput()

		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command timed out after %v", timeout)
		}

		if err != nil {
			return fmt.Errorf("command execution failed: %w", err)
		}

		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("failed to write output to file: %w", err)
		}
		fmt.Printf("Output written to: %s\n", outputFile)
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()

		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command timed out after %v", timeout)
		}

		if err != nil {
			return fmt.Errorf("command execution failed: %w", err)
		}
	}

	return nil
}

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name: "exec",
				Action: func(c *cli.Context) error {
					return execCommand(c, isTorProxyRunning, verifyTorConnection)
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

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
