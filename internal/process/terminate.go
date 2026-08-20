package process

import (
	"os"
	"time"

	gopsutilprocess "github.com/shirou/gopsutil/v4/process"
)

// TerminateWorktreeProcesses finds every process whose cwd is within the given
// worktree path and terminates them.
//
// On unix it sends SIGTERM, waits up to gracePeriod for processes to exit,
// then SIGKILLs any survivors. On windows it uses TerminateProcess.
//
// Returns the list of processes that were targeted. Errors only if the initial
// scan fails; individual kill failures (e.g. process already gone) are
// swallowed.
func TerminateWorktreeProcesses(worktreePath string, gracePeriod time.Duration) ([]ProcessInfo, error) {
	procs, err := FindProcessesInWorktree(worktreePath)
	if err != nil {
		return nil, err
	}
	procs = filterProtectedProcesses(procs, int32(os.Getpid()), parentPID)
	if len(procs) == 0 {
		return nil, nil
	}

	pids := make([]int32, len(procs))
	for i, p := range procs {
		pids[i] = p.PID
	}

	terminate(pids, gracePeriod)
	return procs, nil
}

// ExcludeCurrentProcessAncestry removes the running Treehouse command and its
// ancestors from a process scan. A caller's shell may legitimately have its
// cwd in the worktree while invoking a lifecycle command; unrelated processes
// remain in the result. Parent lookup errors are returned so safety checks can
// fail closed.
func ExcludeCurrentProcessAncestry(procs []ProcessInfo) ([]ProcessInfo, error) {
	return excludeProcessAncestry(procs, int32(os.Getpid()), parentPID)
}

func filterProtectedProcesses(procs []ProcessInfo, currentPID int32, lookupParent func(int32) (int32, error)) []ProcessInfo {
	filtered, err := excludeProcessAncestry(procs, currentPID, lookupParent)
	if err != nil {
		return nil
	}
	return filtered
}

func excludeProcessAncestry(procs []ProcessInfo, currentPID int32, lookupParent func(int32) (int32, error)) ([]ProcessInfo, error) {
	protected := map[int32]struct{}{
		currentPID: {},
	}

	for pid := currentPID; pid > 0; {
		parent, err := lookupParent(pid)
		if err != nil {
			return nil, err
		}
		if parent <= 0 {
			break
		}
		if _, seen := protected[parent]; seen {
			break
		}
		protected[parent] = struct{}{}
		pid = parent
	}

	filtered := procs[:0]
	for _, proc := range procs {
		if _, skip := protected[proc.PID]; skip {
			continue
		}
		filtered = append(filtered, proc)
	}
	return filtered, nil
}

func parentPID(pid int32) (int32, error) {
	proc, err := gopsutilprocess.NewProcess(pid)
	if err != nil {
		return 0, err
	}
	return proc.Ppid()
}
