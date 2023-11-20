package main

type ErrorWithExitCode struct {
	Err      error
	ExitCode int
}

func (e *ErrorWithExitCode) Error() string {
	return e.Err.Error()
}

func (e *ErrorWithExitCode) Unwrap() error {
	return e.Err
}
