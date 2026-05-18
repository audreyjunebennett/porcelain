package config

import "fmt"

// ExitError carries a process exit code for main.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "exit"
}

// Exitf returns an ExitError with the given code and formatted message.
func Exitf(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}
