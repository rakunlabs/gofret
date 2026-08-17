package gofret

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Sentinel errors. Every error produced by gofret wraps one of these, so
// callers can branch with errors.Is instead of matching on strings.
var (
	// ErrSkip is returned by a Hook that does not handle the value. It means
	// "not mine, keep going" and is never reported to the caller. Any other
	// error from a hook aborts the conversion.
	ErrSkip = errors.New("gofret: skip hook")

	// ErrNotPointer is returned by ToInto when out is not a non-nil pointer.
	ErrNotPointer = errors.New("gofret: output must be a non-nil pointer")

	// ErrUnconvertible means the source value cannot be represented in the
	// destination type.
	ErrUnconvertible = errors.New("gofret: unconvertible value")

	// ErrUnsupportedType means the destination type is one gofret cannot
	// write to, such as a channel.
	ErrUnsupportedType = errors.New("gofret: unsupported type")

	// ErrOverflow means the source value does not fit in the destination.
	ErrOverflow = errors.New("gofret: value overflows destination type")

	// ErrUnusedKeys is reported when WithErrorUnused is set and the input
	// contains keys that match no destination field.
	ErrUnusedKeys = errors.New("gofret: unused keys in input")

	// ErrInvalidTag means a struct tag carries an unknown option.
	ErrInvalidTag = errors.New("gofret: invalid struct tag")
)

// Error describes a single conversion failure and where it happened.
//
// Errors are collected rather than aborting on the first failure (unless
// WithFailFast is set) and joined with errors.Join, so errors.Is and
// errors.As reach every one of them.
type Error struct {
	// Path is the dotted location of the value, for example
	// "database.hosts[2].port". It is empty at the root.
	Path string
	// From is the type of the source value. It may be nil for a nil source.
	From reflect.Type
	// To is the type of the destination.
	To reflect.Type
	// Err is the underlying cause.
	Err error
}

func (e *Error) Error() string {
	var sb strings.Builder

	sb.WriteString("gofret: ")

	if e.Path != "" {
		sb.WriteString(e.Path)
		sb.WriteString(": ")
	}

	if e.From != nil && e.To != nil {
		fmt.Fprintf(&sb, "cannot convert %s to %s: ", e.From, e.To)
	}

	sb.WriteString(strings.TrimPrefix(e.Err.Error(), "gofret: "))

	return sb.String()
}

func (e *Error) Unwrap() error { return e.Err }

// Errors returns every *Error in err's tree, in traversal order.
//
// errors.AsType finds the first one. Because a conversion reports all of its
// failures at once, this is the way to list every bad value:
//
//	for _, ce := range gofret.Errors(err) {
//	    log.Printf("%s: %v", ce.Path, ce.Err)
//	}
func Errors(err error) []*Error {
	var out []*Error

	appendErrors(&out, err)

	return out
}

func appendErrors(out *[]*Error, err error) {
	for err != nil {
		if ce, ok := err.(*Error); ok {
			*out = append(*out, ce)
		}

		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, sub := range x.Unwrap() {
				appendErrors(out, sub)
			}

			return
		default:
			return
		}
	}
}

// newError builds an *Error, keeping the call sites terse.
func newError(path string, from, to reflect.Type, err error) *Error {
	return &Error{Path: path, From: from, To: to, Err: err}
}

// errorList accumulates conversion errors, honouring the fail-fast and
// max-errors settings.
type errorList struct {
	errs     []error
	max      int
	failFast bool
	// stopped records that collection hit the limit, so the joined error can
	// say so instead of silently truncating.
	stopped bool
}

// add records err and reports whether collection should stop.
func (l *errorList) add(err error) bool {
	if err == nil {
		return false
	}

	if l.stopped {
		return true
	}

	l.errs = append(l.errs, err)

	if l.failFast {
		l.stopped = true

		return true
	}

	if l.max > 0 && len(l.errs) >= l.max {
		l.stopped = true

		return true
	}

	return false
}

func (l *errorList) empty() bool { return len(l.errs) == 0 }

// err joins the collected errors. It returns nil when nothing was recorded.
func (l *errorList) err() error {
	switch len(l.errs) {
	case 0:
		return nil
	case 1:
		return l.errs[0]
	default:
		return errors.Join(l.errs...)
	}
}
