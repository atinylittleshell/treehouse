package pool

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kunchenguid/treehouse/internal/deadline"
)

// holdStateLock takes the pool lock through a second open file description and
// returns a release function. On both Linux and Windows the lock is a property
// of the open file description, not of the process, so this contends with
// WithStateLock exactly as a separate treehouse process would.
func holdStateLock(t *testing.T, poolDir string) func() {
	t.Helper()
	if err := os.MkdirAll(poolDir, 0o755); err != nil {
		t.Fatalf("creating pool dir: %v", err)
	}
	f, err := os.OpenFile(lockFilePath(poolDir), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("opening lock file: %v", err)
	}
	locked, err := tryLockFile(f)
	if err != nil || !locked {
		f.Close()
		t.Fatalf("could not take the lock to hold it: locked=%v err=%v", locked, err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_ = unlockFile(f)
			_ = f.Close()
		})
	}
	t.Cleanup(release)
	return release
}

// The wedge this pins: before the deadline existed, WithStateLock called
// flock(LOCK_EX) with no bound, so a caller queued behind a stalled holder
// waited forever. In the field that produced treehouse processes sleeping for
// days. The caller's own timeout cannot help - killing the caller leaves the
// treehouse process holding the wait.
func TestWithStateLock_FailsByDeadlineWhenHeldElsewhere(t *testing.T) {
	poolDir := t.TempDir()
	holdStateLock(t, poolDir)

	deadline.Set(300 * time.Millisecond)
	t.Cleanup(func() { deadline.Set(0) })

	ran := false
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		done <- WithStateLock(poolDir, func() error {
			ran = true
			return nil
		})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a deadline error while the lock was held elsewhere")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("error should name the timeout, got: %v", err)
		}
		if !strings.Contains(err.Error(), lockFilePath(poolDir)) {
			t.Fatalf("error should name the lock it waited on, got: %v", err)
		}
		if ran {
			t.Fatal("the critical section must not run without the lock")
		}
		if elapsed := time.Since(started); elapsed > 10*time.Second {
			t.Fatalf("gave up after %s, far past the deadline", elapsed)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("WithStateLock blocked past its deadline: the wedge is back")
	}
}

// A bounded wait must still be a wait: contention is normal, and a caller that
// times out on a holder about to finish would be a worse bug than the wedge.
func TestWithStateLock_WaitsForAHolderThatFinishesInTime(t *testing.T) {
	poolDir := t.TempDir()
	release := holdStateLock(t, poolDir)

	deadline.Set(10 * time.Second)
	t.Cleanup(func() { deadline.Set(0) })

	go func() {
		time.Sleep(200 * time.Millisecond)
		release()
	}()

	ran := false
	done := make(chan error, 1)
	go func() {
		done <- WithStateLock(poolDir, func() error {
			ran = true
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected the lock once the holder released it: %v", err)
		}
		if !ran {
			t.Fatal("the critical section should have run")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("WithStateLock did not pick the lock back up after it was released")
	}
}

// With no deadline set, blocking is unbounded exactly as it always was; the
// bound is opt-in through the deadline package, which only the CLI arms.
func TestWithStateLock_UnboundedWithoutADeadline(t *testing.T) {
	poolDir := t.TempDir()
	release := holdStateLock(t, poolDir)

	deadline.Set(0)

	done := make(chan error, 1)
	go func() { done <- WithStateLock(poolDir, func() error { return nil }) }()

	select {
	case err := <-done:
		t.Fatalf("an unbounded wait must not give up while the lock is held: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected success after release: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("unbounded wait never acquired the released lock")
	}
}

// The holder pid is the difference between "treehouse is stuck" and "pid N is
// holding the pool". It is a diagnostic only, so it is recorded best effort.
func TestWithStateLock_RecordsHolderPID(t *testing.T) {
	poolDir := t.TempDir()

	if err := WithStateLock(poolDir, func() error { return nil }); err != nil {
		t.Fatalf("WithStateLock failed: %v", err)
	}

	data, err := os.ReadFile(lockFilePath(poolDir))
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("lock file should hold the holder pid, got %q: %v", string(data), err)
	}
	if pid != os.Getpid() {
		t.Fatalf("expected pid %d in the lock file, got %d", os.Getpid(), pid)
	}
}

// The incident shape, pinned: a burst of concurrent acquisitions against a pool
// whose lock is held by something that is not making progress. Every client
// must give up by its own deadline rather than accumulate, and the pool must
// still hand out worktrees once the holder is gone - the field evidence was
// that the slots themselves were fine the whole time.
func TestAcquire_BurstBehindAStalledHolderFailsByDeadlineAndPoolStaysGrantable(t *testing.T) {
	repoDir, poolDir := setupRepo(t)
	release := holdStateLock(t, poolDir)

	deadline.Set(500 * time.Millisecond)
	t.Cleanup(func() { deadline.Set(0) })

	const burst = 8
	errs := make(chan error, burst)
	var wg sync.WaitGroup
	for range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := AcquireWithOptions(repoDir, poolDir, 4, nil, AcquireOptions{SkipFetch: true})
			errs <- err
		}()
	}

	settled := make(chan struct{})
	go func() { wg.Wait(); close(settled) }()

	select {
	case <-settled:
	case <-time.After(30 * time.Second):
		t.Fatal("acquisitions survived past their deadline: the wedge is back")
	}

	close(errs)
	for err := range errs {
		if err == nil {
			t.Fatal("an acquisition cannot succeed while the pool lock is held")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected every client to give up on its deadline, got: %v", err)
		}
	}

	// The pool was never damaged, only unreachable. Once the holder lets go it
	// serves acquisitions again.
	release()
	deadline.Set(30 * time.Second)
	wtPath, err := AcquireWithOptions(repoDir, poolDir, 4, nil, AcquireOptions{SkipFetch: true})
	if err != nil {
		t.Fatalf("pool should still be grantable after the holder released: %v", err)
	}
	if wtPath == "" {
		t.Fatal("expected a worktree path")
	}
}
