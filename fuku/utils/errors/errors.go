package errors

import (
	"fmt"
	"runtime"
	"strings"
)

type WrappedError struct {
	Err      error
	Message  string
	File     string
	Line     int
	Function string
}

func (e *WrappedError) Error() string {
	return fmt.Sprintf("%s at %s:%d in %s: %v", e.Message, e.File, e.Line, e.Function, e.Err)
}

func (e *WrappedError) Unwrap() error {
	return e.Err
}

// Wrap wraps err with message and the location of its caller.
func Wrap(err error, message string) error {
	return wrapAt(err, message, 2)
}

// Wrapf is Wrap with a formatted message. It reports its own caller, not Wrap (I6).
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return wrapAt(err, fmt.Sprintf(format, args...), 2)
}

// wrapAt builds the wrapped error; skip counts the frames above it to the caller to report, as
// runtime.Caller counts them (2 is the caller of the exported function that called wrapAt).
func wrapAt(err error, message string, skip int) error {
	if err == nil {
		return nil
	}

	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return fmt.Errorf("%s: %w", message, err)
	}

	fn := runtime.FuncForPC(pc)
	funcName := "unknown"
	if fn != nil {
		funcName = fn.Name()
		parts := strings.Split(funcName, "/")
		if len(parts) > 0 {
			funcName = parts[len(parts)-1]
		}
	}

	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}

	return &WrappedError{
		Err:      err,
		Message:  message,
		File:     file,
		Line:     line,
		Function: funcName,
	}
}
