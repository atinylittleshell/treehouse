package gitvcs

import (
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kunchenguid/treehouse/internal/deadline"
)

// silentOrigin listens on loopback and accepts connections without ever
// speaking the git protocol. It is the reproduction of the field failure: an
// origin that is reachable but never answers, against which `git fetch` waits
// forever. No network access is involved.
func silentOrigin(t *testing.T) string {
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

func repoWithOrigin(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", "."},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
		{"remote", "add", "origin", url},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

// Before the deadline existed, this call never returned. Killing the treehouse
// process did not help either: its `git fetch` child was reparented to init and
// kept running, which is why a caller-side timeout could not clean up after
// itself. Binding the command to the deadline context both bounds the wait and
// kills the child.
func TestFetch_AgainstASilentOriginFailsByDeadline(t *testing.T) {
	repo := repoWithOrigin(t, silentOrigin(t))

	deadline.Set(2 * time.Second)
	t.Cleanup(func() { deadline.Set(0) })

	done := make(chan error, 1)
	started := time.Now()
	go func() { done <- Fetch(repo) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a fetch from an origin that never answers cannot succeed")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected a timeout error, got: %v", err)
		}
		if elapsed := time.Since(started); elapsed > 30*time.Second {
			t.Fatalf("fetch returned after %s, far past its deadline", elapsed)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("fetch blocked past its deadline: the wedge is back")
	}
}

// A command that finishes well inside the budget is untouched, so the bound
// costs nothing on the healthy path.
func TestRunGit_UnaffectedByAGenerousDeadline(t *testing.T) {
	repo := repoWithOrigin(t, "https://example.invalid/none.git")

	deadline.Set(time.Minute)
	t.Cleanup(func() { deadline.Set(0) })

	if _, err := runGit(repo, "rev-parse", "HEAD"); err != nil {
		t.Fatalf("a fast command must not be affected by the deadline: %v", err)
	}
}
