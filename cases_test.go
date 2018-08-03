package tagflag

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.EqualValues(t, err, actualErr)
		},
	}
}

func anyErrorCase(args ...string) parseCase {
	return parseCase{
		args: args,
		err: func(t *testing.T, err error) {
			assert.Error(t, err)
		},
	}
}

func (me parseCase) Run(t *testing.T, newCmd func() interface{}) {
	cmd := newCmd()
	err := ParseErr(cmd, me.args)
	if me.err == nil {
		assert.NoError(t, err)
		assert.EqualValues(t, me.expected, reflect.ValueOf(cmd).Elem().Interface(), "%v", me)
	} else {
		me.err(t, err)
	}
}

func RunCases(t *testing.T, cases []parseCase, newCmd func() interface{}) {
	for _, _case := range cases {
		_case.Run(t, newCmd)
	}
}
