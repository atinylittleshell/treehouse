//go:build !windows

package process

func processIDsInScope(pids []int32, _ int32) (map[int32]bool, error) {
	result := make(map[int32]bool, len(pids))
	for _, pid := range pids {
		result[pid] = true
	}
	return result, nil
}
