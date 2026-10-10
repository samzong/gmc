package worktree

type CreateResult struct {
	Name     string
	Path     string
	Branch   string
	Base     string
	Created  bool
	Err      error
	Warnings []string
}

type AddResult struct {
	CreateResult
	Report Report
}

func (r Report) Warnings() []string {
	var warnings []string
	for _, event := range r.Events {
		if event.Level == EventWarn {
			warnings = append(warnings, event.Message)
		}
	}
	return warnings
}
