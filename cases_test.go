package tagflag

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type parseCase struct {
	args     []string
	err      error
	expected interface{}
}

func noErrorCase(expected interface{}, args ...string) parseCase {
	return parseCase{args: args, expected: expected}
}

func errorCase(err error, args ...string) parseCase {
	return parseCase{args: args, err: err}
}

func (me parseCase) Run(t *testing.T, newCmd func() interface{}) {
	cmd := newCmd()
	err := ParseErr(cmd, me.args)
	assert.EqualValues(t, me.err, err)
	if me.err != nil {
		return
	}
	assert.EqualValues(t, me.expected, reflect.ValueOf(cmd).Elem().Interface(), "%v", me)
}

func RunCases(t *testing.T, cases []parseCase, newCmd func() interface{}) {
	for _, _case := range cases {
		_case.Run(t, newCmd)
	}
}
