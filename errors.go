package router

// FIXME this is unused at present - delete it or use it

// StatusError defines an interface used by Error below
type StatusError interface {
	String() string
	Status() int
}

// Error holds a string and a status for http errors
type Error struct {
	message string
	status  int
}

// Status returns the error status
func (e *Error) Status() int {
	return e.status
}

// String returns a string representation of the error
func (e *Error) String() string {
	return e.message
}

// NewError returns a new error
func NewError(m string, s int) *Error {
	return &Error{
		message: m,
		status:  s,
	}
}
