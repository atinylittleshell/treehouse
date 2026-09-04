// Package deadline carries the single process-wide bound on how long one
// treehouse command may block.
//
// Treehouse blocks in exactly three places: the pool state flock, the git
// subprocess runner, and the jj subprocess runner. Before this package all
// three were unbounded, so a stalled origin or a stalled lock holder parked a
// `treehouse get` forever - observed in the field as processes sleeping for
// days while the pool itself was healthy. A caller's own timeout cannot fix
// that: killing the caller leaves the treehouse process, and killing the
// treehouse process leaves its `git fetch` child reparented to init. The bound
// has to be enforced by the process that owns the wait.
//
// The deadline is process-wide rather than a context.Context threaded through
// the call graph on purpose. A treehouse process runs exactly one command, and
// the three blocking points sit behind vcs.Backend, a 19-operation interface;
// giving every operation and every caller a context parameter would be a large
// diff whose only consumer is these three call sites.
//
// The budget is per PHASE of work, not per process. Three primitives block for
// an arbitrarily long time on something outside treehouse's control - the
// interactive subshell (shell.Spawn), user-configured lifecycle hooks
// (hooks.Run), and a yes/no prompt to a person (ui.Confirm) - and none of them
// is bounded, on purpose. Each one calls Restart on the way out, so the time it
// spent is not charged to whatever runs next. That rule lives in those three
// functions rather than at their call sites: a caller can forget, and forgetting
// means a healthy operation fails on an expired deadline. A new unbounded
// primitive belongs on that list.
package deadline

import (
	"context"
	"sync/atomic"
	"time"
)

// Default bounds a command that did not ask for a specific timeout. It is set
// far above any healthy operation - a cold fetch of a large repository is
// seconds to low minutes - so that it only ever fires on a genuine stall.
const Default = 10 * time.Minute

// deadline holds the absolute time this process must stop waiting, or nil for
// an unbounded process.
var deadline atomic.Pointer[time.Time]

// budget remembers the duration Set was last called with, so Restart can grant
// a fresh one without the caller having to re-resolve it.
var budget atomic.Int64

// Set bounds the rest of this process to d from now. A non-positive d clears
// the bound and restores unbounded blocking.
func Set(d time.Duration) {
	budget.Store(int64(d))
	if d <= 0 {
		deadline.Store(nil)
		return
	}
	at := time.Now().Add(d)
	deadline.Store(&at)
}

// Restart grants a fresh budget of the duration last passed to Set.
//
// The bound is per phase of work, not per process. A command that hands control
// to something deliberately unbounded - the subshell `treehouse get` opens, and
// only that - would otherwise come back to an expired deadline and fail its
// cleanup instantly. Such a command calls Restart when it takes control back.
func Restart() {
	Set(time.Duration(budget.Load()))
}

// At returns the absolute deadline for this process, if one is set.
func At() (time.Time, bool) {
	at := deadline.Load()
	if at == nil {
		return time.Time{}, false
	}
	return *at, true
}

// Exceeded reports whether the deadline has already passed.
func Exceeded() bool {
	at, ok := At()
	return ok && !time.Now().Before(at)
}

// Context returns a context bounded by the process deadline. The caller must
// call the returned cancel function. With no deadline set the context is
// context.Background with a no-op cancel, so callers need no special case.
func Context() (context.Context, context.CancelFunc) {
	at, ok := At()
	if !ok {
		return context.Background(), func() {}
	}
	return context.WithDeadline(context.Background(), at)
}
