package cmd

import (
	"net"
	"strings"
	"testing"
	"time"
)

// silentOriginURL listens on loopback and accepts connections without ever
// speaking the git protocol, so a fetch against it waits forever. No network
// access is involved.
func silentOriginURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		var held []net.Conn
		defer func() {
			for _, c := range held {
				_ = c.Close()
			}
		}()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, c)
		}
	}()
	return "git://" + ln.Addr().String() + "/silent.git"
}

// The end-to-end contract the incident asks for: a `treehouse get` that cannot
// make progress exits by its own deadline instead of parking indefinitely.
// Before this, the process outlived every timeout its caller applied, because
// no timeout the caller owns can reach a wait inside another process.
func TestGetExitsByItsOwnTimeoutWhenOriginNeverAnswers(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	gitCmd(t, repoDir, "remote", "set-url", "origin", silentOriginURL(t))

	done := make(chan struct{})
	var stderr string
	var exitCode int
	go func() {
		defer close(done)
		_, stderr, exitCode = runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--timeout", "2s")
	}()

	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("treehouse get blocked past its own --timeout: the wedge is back")
	}

	if exitCode == 0 {
		t.Fatalf("expected a non-zero exit when the fetch cannot complete; stderr: %s", stderr)
	}
	if !strings.Contains(stderr, "timed out") {
		t.Fatalf("expected the timeout to be reported, got: %s", stderr)
	}
}

// TREEHOUSE_TIMEOUT is the same bound for callers that cannot add a flag, and
// the flag wins over it.
func TestGetTimeoutFromEnvironment(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	gitCmd(t, repoDir, "remote", "set-url", "origin", silentOriginURL(t))

	done := make(chan struct{})
	var stderr string
	var exitCode int
	go func() {
		defer close(done)
		_, stderr, exitCode = runTreehouse(t, repoDir, homeDir, []string{"TREEHOUSE_TIMEOUT=2s"}, "get", "--lease")
	}()

	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("TREEHOUSE_TIMEOUT was not honored")
	}

	if exitCode == 0 {
		t.Fatalf("expected a non-zero exit; stderr: %s", stderr)
	}
	if !strings.Contains(stderr, "timed out") {
		t.Fatalf("expected the timeout to be reported, got: %s", stderr)
	}
}

// The bound must not cost anything on the healthy path: a normal acquire with
// the default timeout in force still succeeds.
func TestGetSucceedsWithTheDefaultTimeout(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	stdout, stderr, exitCode := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease")
	if exitCode != 0 {
		t.Fatalf("expected a healthy acquire to succeed, exit %d: %s", exitCode, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatalf("expected a worktree path on stdout; stderr: %s", stderr)
	}
}
