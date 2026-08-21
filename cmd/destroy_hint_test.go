package cmd

import (
	"strings"
	"testing"

	"github.com/kunchenguid/treehouse/internal/pool"
)

// TestDestroySkipHintFramesOtherFlavorAsMigration pins the skip wording: an
// other-flavor worktree whose only unlanded class is unverified holds work
// nothing proved unlanded, so the hint frames --include-unlanded as the pool
// migration step. A genuinely dirty other-flavor worktree keeps the plain
// data-loss hint.
func TestDestroySkipHintFramesOtherFlavorAsMigration(t *testing.T) {
	skip := pool.DestroySkip{
		Target: pool.DestroyTarget{
			Name:        "1",
			Flavor:      "git",
			OtherFlavor: true,
			Classes:     []pool.DestroyClass{pool.DestroyUnverified},
		},
		NeededFlag:  pool.IncludeUnlandedFlag,
		NeededFlags: []string{pool.IncludeUnlandedFlag},
	}
	hint := destroySkipHint(skip)
	if !strings.Contains(hint, "migrate") || !strings.Contains(hint, pool.IncludeUnlandedFlag) {
		t.Fatalf("expected a migration-framed hint naming the flag, got %q", hint)
	}

	skip.Target.Classes = []pool.DestroyClass{pool.DestroyDirty}
	if hint := destroySkipHint(skip); strings.Contains(hint, "migrate") {
		t.Fatalf("a dirty worktree must keep the data-loss hint, got %q", hint)
	}

	skip.Target.Classes = []pool.DestroyClass{pool.DestroyUnverified}
	skip.Target.OtherFlavor = false
	if hint := destroySkipHint(skip); strings.Contains(hint, "migrate") {
		t.Fatalf("a same-flavor worktree must keep the plain hint, got %q", hint)
	}
}
