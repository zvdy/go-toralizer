package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeChecker struct {
	reachableErr, verifyErr error
	verifyCalls             int
}

func (f *fakeChecker) Reachable(context.Context, string) error { return f.reachableErr }

func (f *fakeChecker) Verify(context.Context, string) error {
	f.verifyCalls++
	return f.verifyErr
}

func TestExec(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		checker     *fakeChecker
		wantErr     string
		wantExit    int
		wantVerifys int
	}{
		{name: "success", args: []string{"-c", "true"}, checker: &fakeChecker{}, wantVerifys: 1},
		{name: "skip verify", args: []string{"--skip-verify", "-c", "true"}, checker: &fakeChecker{}},
		{name: "argv form", args: []string{"--skip-verify", "--", "true"}, checker: &fakeChecker{}},
		{name: "proxy down", args: []string{"-c", "true"}, checker: &fakeChecker{reachableErr: errors.New("refused")}, wantErr: "not reachable"},
		{name: "verify fails", args: []string{"-c", "true"}, checker: &fakeChecker{verifyErr: errors.New("not tor")}, wantErr: "verification failed", wantVerifys: 1},
		{name: "no command", args: nil, checker: &fakeChecker{}, wantErr: "provide either"},
		{name: "both forms", args: []string{"-c", "true", "--", "true"}, checker: &fakeChecker{}, wantErr: "provide either"},
		{name: "exit code", args: []string{"--skip-verify", "--mode", "env", "-c", "exit 7"}, checker: &fakeChecker{}, wantExit: 7},
		{name: "bad scheme", args: []string{"-p", "http://127.0.0.1:8080", "-c", "true"}, checker: &fakeChecker{}, wantErr: "socks5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newApp(tt.checker).Run(context.Background(), append([]string{"go-toralizer", "exec"}, tt.args...))
			var ee exitError
			switch {
			case tt.wantExit != 0:
				if !errors.As(err, &ee) || ee.code != tt.wantExit {
					t.Fatalf("err = %v, want exit %d", err, tt.wantExit)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checker.verifyCalls != tt.wantVerifys {
				t.Errorf("verify calls = %d, want %d", tt.checker.verifyCalls, tt.wantVerifys)
			}
		})
	}
}
