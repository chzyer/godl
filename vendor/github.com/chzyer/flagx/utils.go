package flagx

import (
	"errors"
	"fmt"
	"strings"
)

func IsPubField(name string) bool {
	rs := []rune(name)
	return strings.ToUpper(string(rs[0])) == string(rs[0])
}

type ErrorFmt struct {
	err  error
	args []interface{}
}

func Error(msg string) *ErrorFmt {
	return &ErrorFmt{err: errors.New(msg)}
}

func (e *ErrorFmt) Format(args ...interface{}) error {
	e.args = args
	return e
}

func (e *ErrorFmt) Formatf(fmt_ string, args ...interface{}) error {
	return e.Format(fmt.Sprintf(fmt_, args...))
}

func (e *ErrorFmt) Error() string {
	return fmt.Sprintf(e.err.Error(), e.args...)
}
