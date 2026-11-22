package vkmauth

import "fmt"

func newUnknownError(statusCode int, at string, w map[string]any) UnknownError {
	return UnknownError{
		StatusCode: statusCode,
		At:         at,
		Wrap:       w,
	}
}

type UnknownError struct {
	StatusCode int
	At         string
	Wrap       map[string]any
}

func (e UnknownError) Error() string {
	return fmt.Sprintf("status %d, at %s (unwrap UnknownError for more info)", e.StatusCode, e.At)
}

func newError(msg string, at string) error {
	return fmt.Errorf("%s: %s", msg, at)
}
