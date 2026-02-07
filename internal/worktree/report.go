package worktree

type EventLevel string

const (
	EventInfo EventLevel = "info"
	EventWarn EventLevel = "warn"
)

type Event struct {
	Level   EventLevel
	Message string
}

type Report struct {
	Events []Event
}

func (r *Report) Add(level EventLevel, message string) {
	r.Events = append(r.Events, Event{
		Level:   level,
		Message: message,
	})
}

func (r *Report) Info(message string) {
	r.Add(EventInfo, message)
}

func (r *Report) Warn(message string) {
	r.Add(EventWarn, message)
}

func (r *Report) Merge(other Report) {
	if len(other.Events) == 0 {
		return
	}
	r.Events = append(r.Events, other.Events...)
}
