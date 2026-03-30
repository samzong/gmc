package exitcode

const (
	Success         = 0
	General         = 1
	Usage           = 2
	NoStagedChanges = 10
	NotGitRepo      = 11
	LLMError        = 12
)

type Error struct {
	Code    int
	Message string
	Err     error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

func New(code int, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}
