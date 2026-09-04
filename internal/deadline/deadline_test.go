package deadline

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUnsetIsUnbounded(t *testing.T) {
	t.Cleanup(func() { Set(0) })

	Set(0)
	if _, ok := At(); ok {
		t.Fatal("a non-positive duration must clear the deadline")
	}
	if Exceeded() {
		t.Fatal("an unbounded process is never past its deadline")
	}
	ctx, cancel := Context()
	defer cancel()
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("an unbounded process must hand out a context with no deadline")
	}
}

func TestExceededAfterBudgetElapses(t *testing.T) {
	t.Cleanup(func() { Set(0) })

	Set(20 * time.Millisecond)
	if Exceeded() {
		t.Fatal("a fresh budget must not read as exceeded")
	}
	time.Sleep(50 * time.Millisecond)
	if !Exceeded() {
		t.Fatal("an elapsed budget must read as exceeded")
	}

	ctx, cancel := Context()
	defer cancel()
	if err := ctx.Err(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected an expired context, got %v", err)
	}
}

// Restart is what keeps a long interactive subshell from poisoning the cleanup
// that follows it: the budget is per phase of work, not per process.
func TestRestartGrantsAFreshBudget(t *testing.T) {
	t.Cleanup(func() { Set(0) })

	Set(20 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	if !Exceeded() {
		t.Fatal("precondition: budget should have elapsed")
	}

	Restart()
	if Exceeded() {
		t.Fatal("Restart must grant a fresh budget of the same duration")
	}
}

func TestRestartKeepsAnUnboundedProcessUnbounded(t *testing.T) {
	t.Cleanup(func() { Set(0) })

	Set(0)
	Restart()
	if _, ok := At(); ok {
		t.Fatal("Restart must not invent a deadline for an unbounded process")
	}
}
