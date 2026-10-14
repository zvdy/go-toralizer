package runner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunEnv(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com")
	t.Setenv("HTTP_PROXY", "http://old:1")
	var out, errb bytes.Buffer
	code, err := Run(context.Background(), Options{
		Argv:   []string{"env"},
		Proxy:  "socks5h://127.0.0.1:9150",
		Mode:   ModeEnv,
		Stdout: &out,
		Stderr: &errb,
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	lines := strings.Split(out.String(), "\n")
	has := func(s string) bool {
		for _, l := range lines {
			if l == s {
				return true
			}
		}
		return false
	}
	for _, k := range proxyVars {
		if !has(k + "=socks5h://127.0.0.1:9150") {
			t.Errorf("missing %s", k)
		}
	}
	for _, k := range noProxyVars {
		if has(k + "=example.com") {
			t.Errorf("%s not cleared", k)
		}
	}
	if has("HTTP_PROXY=http://old:1") {
		t.Error("old HTTP_PROXY retained")
	}
}

func TestRunValidation(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"neither", Options{}},
		{"both", Options{Argv: []string{"true"}, Shell: "true"}},
		{"bad mode", Options{Argv: []string{"true"}, Mode: "nope"}},
		{"bad proxy", Options{Argv: []string{"true"}, Proxy: "http://127.0.0.1:8080"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Run(context.Background(), tt.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRunExitCodeAndShell(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want int
		out  string
	}{
		{"argv exit 3", Options{Argv: []string{"sh", "-c", "exit 3"}}, 3, ""},
		{"shell exit 7", Options{Shell: "echo hi; exit 7"}, 7, "hi\n"},
		{"argv no shell", Options{Argv: []string{"echo", "$HOME;x"}}, 0, "$HOME;x\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			tt.opts.Mode = ModeEnv
			tt.opts.Stdout = &out
			tt.opts.Stderr = &bytes.Buffer{}
			code, err := Run(context.Background(), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if code != tt.want || out.String() != tt.out {
				t.Errorf("code=%d out=%q", code, out.String())
			}
		})
	}
}

func TestRunTimeout(t *testing.T) {
	start := time.Now()
	_, err := Run(context.Background(), Options{
		Argv:    []string{"sleep", "10"},
		Mode:    ModeEnv,
		Timeout: 100 * time.Millisecond,
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out after 100ms") {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("timeout not enforced promptly")
	}
}

func TestRunTorsocksMissing(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := Run(context.Background(), Options{
		Argv:   []string{"true"},
		Mode:   ModeTorsocks,
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "torsocks") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunAutoFallbackWarns(t *testing.T) {
	dir := t.TempDir()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found")
	}
	if err := os.Symlink(sh, filepath.Join(dir, "sh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	var errb bytes.Buffer
	code, err := Run(context.Background(), Options{
		Shell:  "exit 0",
		Mode:   ModeAuto,
		Stdout: &bytes.Buffer{},
		Stderr: &errb,
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(errb.String(), "bypass Tor") {
		t.Errorf("no warning: %q", errb.String())
	}
}

func TestRunTorsocksFake(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$TORSOCKS_TOR_ADDRESS:$TORSOCKS_TOR_PORT\"\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "torsocks"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	code, err := Run(context.Background(), Options{
		Shell:  "echo done",
		Proxy:  "10.0.0.1:9999",
		Mode:   ModeTorsocks,
		Stdout: &out,
		Stderr: &bytes.Buffer{},
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if out.String() != "10.0.0.1:9999\ndone\n" {
		t.Errorf("out=%q", out.String())
	}
}

func TestRunOutputFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("stale content that is long"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, err := Run(context.Background(), Options{
		Shell:      "echo out; echo err >&2",
		Mode:       ModeEnv,
		OutputPath: path,
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "out\n") || !strings.Contains(string(got), "err\n") || strings.Contains(string(got), "stale") {
		t.Errorf("file=%q", got)
	}
	if out.String() != "out\n" {
		t.Errorf("stdout=%q", out.String())
	}

	fresh := filepath.Join(dir, "fresh.txt")
	if _, err := Run(context.Background(), Options{Argv: []string{"true"}, Mode: ModeEnv, OutputPath: fresh, Stdout: &out, Stderr: &out}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm=%o", perm)
	}
}

func TestRunOutputSymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		Argv:       []string{"true"},
		Mode:       ModeEnv,
		OutputPath: link,
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "symbolic links") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunVerbose(t *testing.T) {
	t.Setenv("SECRET_TOKEN_FOR_TEST", "hunter2")
	var errb bytes.Buffer
	_, err := Run(context.Background(), Options{
		Argv:    []string{"true"},
		Mode:    ModeEnv,
		Verbose: true,
		Stdout:  &bytes.Buffer{},
		Stderr:  &errb,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := errb.String()
	if !strings.Contains(s, "mode=env") || !strings.Contains(s, "socks5h://127.0.0.1:9050") {
		t.Errorf("log=%q", s)
	}
	if strings.Contains(s, "hunter2") {
		t.Error("environment leaked into log")
	}
}

func TestBuildEnv(t *testing.T) {
	got := buildEnv([]string{"A=1", "B=2"}, []string{"B=3", "C="})
	want := []string{"A=1", "B=3", "C="}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v", got)
	}
}
