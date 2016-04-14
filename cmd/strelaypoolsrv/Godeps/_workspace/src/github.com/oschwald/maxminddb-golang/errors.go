package maxminddb

import "fmt"

// InvalidDatabaseError is returned when the database contains invalid data
// and cannot be parsed.
type InvalidDatabaseError struct {
	message string
}

func newInvalidDatabaseError(format string, args ...interface{}) InvalidDatabaseError {
	return InvalidDatabaseError{fmt.Sprintf(format, args...)}
}

func (e InvalidDatabaseError) Error() string {
	return e.message
}
