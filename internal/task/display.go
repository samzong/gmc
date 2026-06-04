package task

// ListAgentMode returns agent and mode columns for task list (from the primary attempt).
func ListAgentMode(sum Summary) (agent, mode string) {
	agent, mode = "-", "-"
	if len(sum.Attempts) == 0 {
		return agent, mode
	}
	a := sum.Attempts[len(sum.Attempts)-1]
	if a.Agent != "" {
		agent = a.Agent
	}
	if a.Mode != "" {
		mode = a.Mode
	} else if a.Agent != "" {
		mode = "coding"
	}
	return agent, mode
}
