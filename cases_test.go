package tagflag

import (
	"reflect"
	"testing"

	"github.com/go-quicktest/qt"
)

type parseCase struct {
	args     []string
	err      func(*testing.T, error)
	expected interface{}
}

func noErrorCase(expected interface{}, args ...string) parseCase {
	return parseCase{args: args, expected: expected}
}

func errorCase(err error, args ...string) parseCase {
	return parseCase{
		args: args,
		err: func(t *testing.T, actualErr error) {
			qt.Check(t, qt.Equals(actualErr, err))
		},
	}
}

func anyErrorCase(args ...string) parseCase {
	return parseCase{
		args: args,
		err: func(t *testing.T, err error) {
			qt.Check(t, qt.IsNotNil(err))
		},
	}
}

func (me parseCase) Run(t *testing.T, newCmd func() interface{}) {
	cmd := newCmd()
	err := ParseErr(cmd, me.args)
	if me.err == nil {
		qt.Check(t, qt.IsNil(err))
		qt.Check(t,
			qt.DeepEquals[any](reflect.ValueOf(cmd).Elem().Interface(), me.expected),
			qt.Commentf("%v", me))
	} else {
		me.err(t, err)
	}
}

func RunCases(t *testing.T, cases []parseCase, newCmd func() interface{}) {
	for _, _case := range cases {
		_case.Run(t, newCmd)
	}
}
