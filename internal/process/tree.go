package process

import (
	"github.com/shirou/gopsutil/v4/process"
)

// maxDescendantDepth caps the process-tree walk. Nothing treehouse spawns nests
// anywhere near this deep; the cap exists so a cycle or a pathological tree
// cannot turn a best-effort cleanup into an unbounded walk.
const maxDescendantDepth = 8

// KillDescendants kills every live descendant of pid, deepest first. It does
// NOT touch pid itself.
//
// Call it BEFORE killing pid. Once a parent exits its children are reparented
// to init, and there is no longer any way to walk down from the parent to find
// them. That ordering is the whole point: killing a `git` process does not kill
// the transport helper it spawned (`git-remote-http`, `ssh`), which inherits the
// output pipes and keeps holding the repository open.
//
// Best effort throughout. A process that has already exited, or that this user
// may not signal, is skipped rather than reported: the caller is cleaning up
// after a deadline it has already decided to fail on, and nothing here changes
// that outcome.
func KillDescendants(pid int32) {
	for _, proc := range descendants(pid) {
		_ = proc.Kill()
	}
}

// descendants returns the live descendants of pid, deepest first, so children
// are signalled before the parents that could otherwise respawn or outlive them.
func descendants(pid int32) []*process.Process {
	root, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	var found []*process.Process
	var walk func(parent *process.Process, depth int)
	walk = func(parent *process.Process, depth int) {
		if depth >= maxDescendantDepth {
			return
		}
		children, err := parent.Children()
		if err != nil {
			// No children, or the process died mid-walk. Either way there is
			// nothing further down this branch to collect.
			return
		}
		for _, child := range children {
			walk(child, depth+1)
			found = append(found, child)
		}
	}
	walk(root, 0)
	return found
}
