package process

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

type ProcessInfo struct {
	PID  int32
	Name string
}

type processProbe interface {
	PID() int32
	Status() ([]string, error)
	Username() (string, error)
	Cwd() (string, error)
	Name() (string, error)
	Exists() (bool, error)
}

type systemProcessProbe struct {
	process *process.Process
}

func (p systemProcessProbe) PID() int32 {
	return p.process.Pid
}

func (p systemProcessProbe) Status() ([]string, error) {
	return p.process.Status()
}

func (p systemProcessProbe) Username() (string, error) {
	return p.process.Username()
}

func (p systemProcessProbe) Cwd() (string, error) {
	return p.process.Cwd()
}

func (p systemProcessProbe) Name() (string, error) {
	return p.process.Name()
}

func (p systemProcessProbe) Exists() (bool, error) {
	return process.PidExists(p.process.Pid)
}

func (p ProcessInfo) String() string {
	return fmt.Sprintf("%s (%d)", p.Name, p.PID)
}

func IsWorktreeInUse(worktreePath string) (bool, error) {
	procs, err := FindProcessesInWorktree(worktreePath)
	if err != nil {
		return false, err
	}
	return len(procs) > 0, nil
}

func Exists(pid int32) bool {
	exists, err := process.PidExists(pid)
	return err == nil && exists
}

func StartedAt(pid int32) (int64, bool) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return 0, false
	}
	startedAt, err := proc.CreateTime()
	return startedAt, err == nil
}

// FindProcessesInWorktree returns processes whose current directory is the
// worktree root or a descendant after absolute path and symlink resolution.
func FindProcessesInWorktree(worktreePath string) ([]ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	absWorktree, err := filepath.Abs(worktreePath)
	if err != nil {
		return nil, err
	}
	absWorktree = resolvePath(absWorktree)

	var result []ProcessInfo

	for _, p := range procs {
		cwd, err := p.Cwd()
		if err != nil {
			continue
		}

		absCwd, err := filepath.Abs(cwd)
		if err != nil {
			continue
		}
		absCwd = resolvePath(absCwd)

		rel, err := filepath.Rel(absWorktree, absCwd)
		if err != nil {
			continue
		}

		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			name, _ := p.Name()
			result = append(result, ProcessInfo{
				PID:  p.Pid,
				Name: name,
			})
		}
	}

	return result, nil
}

// FindProcessesInWorktreeStrict fails when it cannot inspect a live process
// owned by the current user.
func FindProcessesInWorktreeStrict(worktreePath string) ([]ProcessInfo, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, err
	}
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	pids := make([]int32, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, p.Pid)
	}
	inScope, err := processIDsInScope(pids, int32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	probes := make([]processProbe, 0, len(procs))
	currentPID := int32(os.Getpid())
	for _, p := range procs {
		if shouldSkipProcess(runtime.GOOS, p.Pid, currentPID) || !inScope[p.Pid] {
			continue
		}
		probes = append(probes, systemProcessProbe{process: p})
	}
	return findProcessesInWorktreeStrictForOS(worktreePath, currentUser.Username, runtime.GOOS, probes)
}

func shouldSkipProcess(goos string, pid, currentPID int32) bool {
	return pid == currentPID || (goos == "windows" && pid == 0)
}

func findProcessesInWorktreeStrict(worktreePath, currentUsername string, procs []processProbe) ([]ProcessInfo, error) {
	return findProcessesInWorktreeStrictForOS(worktreePath, currentUsername, runtime.GOOS, procs)
}

func findProcessesInWorktreeStrictForOS(worktreePath, currentUsername, goos string, procs []processProbe) ([]ProcessInfo, error) {
	absWorktree, err := filepath.Abs(worktreePath)
	if err != nil {
		return nil, err
	}
	absWorktree = resolvePath(absWorktree)

	var result []ProcessInfo
	for _, p := range procs {
		if goos == "windows" {
			owned, err := belongsToCurrentUser(p, currentUsername)
			if err != nil {
				return nil, err
			}
			if !owned {
				continue
			}
		}
		cwd, err := p.Cwd()
		if err != nil {
			alive, aliveErr := p.Exists()
			if aliveErr != nil {
				return nil, aliveErr
			}
			if alive && !isZombie(goos, p) &&
				(goos == "windows" || !belongsToAnotherUser(p, currentUsername)) {
				return nil, fmt.Errorf("inspect cwd for process %d: %w", p.PID(), err)
			}
			continue
		}
		if cwd == "" {
			alive, aliveErr := p.Exists()
			if aliveErr != nil {
				return nil, aliveErr
			}
			if alive && !isZombie(goos, p) &&
				(goos == "windows" || !belongsToAnotherUser(p, currentUsername)) {
				return nil, fmt.Errorf("inspect cwd for process %d: empty path", p.PID())
			}
			continue
		}
		absCwd, err := filepath.Abs(cwd)
		if err != nil {
			return nil, err
		}
		absCwd = resolvePath(absCwd)
		if pathContains(absWorktree, absCwd) {
			name, _ := p.Name()
			result = append(result, ProcessInfo{PID: p.PID(), Name: name})
		}
	}
	return result, nil
}

func belongsToCurrentUser(p processProbe, currentUsername string) (bool, error) {
	username, err := p.Username()
	if err == nil {
		return strings.EqualFold(username, currentUsername), nil
	}
	alive, aliveErr := p.Exists()
	if aliveErr != nil {
		return false, aliveErr
	}
	if !alive {
		return false, nil
	}
	return false, fmt.Errorf("inspect owner for process %d: %w", p.PID(), err)
}

func belongsToAnotherUser(p processProbe, currentUsername string) bool {
	username, err := p.Username()
	return err == nil && !strings.EqualFold(username, currentUsername)
}

func isZombie(goos string, p processProbe) bool {
	if goos == "windows" {
		return false
	}
	statuses, err := p.Status()
	return err == nil && containsProcessStatus(statuses, process.Zombie)
}

func containsProcessStatus(statuses []string, want string) bool {
	for _, status := range statuses {
		if status == want {
			return true
		}
	}
	return false
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolvePath returns the symlink-resolved path, or the input if resolution
// fails (e.g. path doesn't exist). This lets us match process cwds (which
// gopsutil returns canonicalized, e.g. /private/var/... on macOS) against
// caller-supplied worktree paths that may still contain symlinks.
func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}
