package process

import "time"

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
