package tagflag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasic(t *testing.T) {
	type simpleCmd struct {
		Verbose bool   `type:"flag" short:"v"`
		Arg     string `type:"pos"`
	}
	for _, _case := range []struct {
		expected simpleCmd
		err      error
		args     []string
	}{
		{
			simpleCmd{true, "test"},
			nil,
			[]string{"--verbose", "test"},
		},
		{
			simpleCmd{false, "hello"},
			nil,
			[]string{"hello"},
		},
		{
			simpleCmd{},
			userError{`excess argument: "world"`},
			[]string{"hello", "world"},
		},
		{
			simpleCmd{},
			userError{`missing argument: "ARG"`},
			[]string{"-v"},
		},
		{
			simpleCmd{},
			userError{`unexpected flag: "-n"`},
			[]string{"-no"},
		},
	} {
		var actual simpleCmd
		err := ParseEx(&actual, _case.args)
		assert.EqualValues(t, _case.err, err)
		if _case.err != nil {
			continue
		}
		assert.EqualValues(t, _case.expected, actual)
	}
}

func TestNotBasic(t *testing.T) {
	type cmd struct {
		Seed       bool
		NoUpload   bool
		ListenAddr string
		DataDir    string   `short:"d"`
		Torrent    []string `type:"pos" arity:"+"`
	}
	for _, _case := range []struct {
		args     []string
		err      error
		expected cmd
	}{
		{nil, userError{`missing argument: "TORRENT"`}, cmd{}},
		{
			[]string{"--seed"},
			userError{`missing argument: "TORRENT"`},
			cmd{},
		},
		{
			[]string{"a.torrent", "--seed", "b.torrent"},
			nil,
			cmd{
				Torrent: []string{"a.torrent", "b.torrent"},
				Seed:    true,
			},
		},
		{
			[]string{"a.torrent", "b.torrent", "--listen-addr", "1.2.3.4:80"},
			nil,
			cmd{
				ListenAddr: "1.2.3.4:80",
				Torrent:    []string{"a.torrent", "b.torrent"},
			},
		},
		{
			[]string{"-d", "/tmp", "a.torrent", "b.torrent", "--listen-addr", "1.2.3.4:80"},
			nil,
			cmd{
				DataDir:    "/tmp",
				ListenAddr: "1.2.3.4:80",
				Torrent:    []string{"a.torrent", "b.torrent"},
			},
		},
		{
			[]string{"--no-upload=true", "a.torrent", "--no-upload=false"},
			nil,
			cmd{
				NoUpload: false,
				Torrent:  []string{"a.torrent"},
			},
		},
	} {
		var actual cmd
		err := ParseEx(&actual, _case.args)
		assert.EqualValues(t, _case.err, err)
		if _case.err != nil {
			continue
		}
		assert.EqualValues(t, _case.expected, actual)
	}
}

func TestBadCommand(t *testing.T) {
	assert.EqualValues(t,
		userError{"cmd must be ptr or nil"},
		ParseEx(struct{}{}, nil))
	assert.NoError(t, ParseEx(new(struct{}), nil))
	assert.NoError(t, ParseEx(nil, nil))
}

func TestVarious(t *testing.T) {
	a := &struct {
		A string `type:"pos" arity:"+"`
	}{}
	t.Log(ParseEx(a, nil))
	t.Log(ParseEx(a, []string{"a"}))
	assert.EqualValues(t, "a", a.A)
	t.Log(ParseEx(a, []string{"a", "b"}))
	assert.EqualValues(t, "b", a.A)
}

func TestBasicPositionalArities(t *testing.T) {
	type cmd struct {
		A string `type:"pos"`
		B int64  `type:"pos" arity:"?"`
		C bool
		D []string `type:"pos" arity:"*"`
	}
	for _, _case := range []struct {
		args     []string
		err      error
		expected cmd
	}{
		{nil, userError{`missing argument: "A"`}, cmd{}},
		{[]string{"abc"}, nil, cmd{A: "abc"}},
		{[]string{"abc", "123"}, nil, cmd{A: "abc", B: 123}},
		{[]string{"abc", "123", "first"}, nil, cmd{A: "abc", B: 123, D: []string{"first"}}},
		{[]string{"abc", "123", "first", "second"}, nil, cmd{A: "abc", B: 123, D: []string{"first", "second"}}},
		{[]string{"abc", "123", "--c", "first", "second"}, nil, cmd{A: "abc", B: 123, C: true, D: []string{"first", "second"}}},
	} {
		var actual cmd
		err := ParseEx(&actual, _case.args)
		assert.EqualValues(t, _case.err, err)
		if _case.err != nil {
			continue
		}
		assert.EqualValues(t, _case.expected, actual)
	}
}
