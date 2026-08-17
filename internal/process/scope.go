package process

func processIDsForSession(currentSession uint32, sessions map[int32]uint32) map[int32]bool {
	result := make(map[int32]bool, len(sessions))
	for pid, session := range sessions {
		if session == currentSession {
			result[pid] = true
		}
	}
	return result
}
