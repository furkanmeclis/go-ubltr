package builder

import (
	"errors"
	"fmt"
)

// ErrBuilder is the sentinel wrapped by [BuilderError].
var ErrBuilder = errors.New("builder")

// BuilderError is a construction failure. It is returned before XML is produced.
type BuilderError struct {
	Field string
	Msg   string
	err   error
}

func (e *BuilderError) Error() string {
	if e == nil {
		return "builder"
	}
	if e.Field != "" {
		return fmt.Sprintf("builder: %s: %s", e.Field, e.Msg)
	}
	return "builder: " + e.Msg
}

func (e *BuilderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *BuilderError) Is(target error) bool {
	return target == ErrBuilder
}

func builderErr(field, msg string) error {
	return &BuilderError{Field: field, Msg: msg}
}

func builderWrap(field string, err error) error {
	if err == nil {
		return nil
	}
	var be *BuilderError
	if errors.As(err, &be) {
		return err
	}
	return &BuilderError{Field: field, Msg: err.Error(), err: err}
}
