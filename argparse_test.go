package argparse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasic(t *testing.T) {
	type simpleCmd struct {
		Verbose bool   `type:"flag" short:"v"`
		Arg     string `type:"arg"`
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
			userError{`unexpected flag: "-no"`},
			[]string{"-no"},
		},
	} {
		var actual simpleCmd
		err := Args(&actual, _case.args)
		if _case.err == nil {
			assert.NoError(t, err)
			assert.EqualValues(t, _case.expected, actual)
		} else {
			assert.EqualValues(t, _case.err, err)
		}
	}
}

func TestNotBasic(t *testing.T) {
	type cmd struct {
		Seed       bool
		NoUpload   bool
		ListenAddr string
		DataDir    string   `short:"d"`
		Torrent    []string `type:"arg" arity:"+"`
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
			[]string{"a.torrent", "b.torrent"},
			nil,
			cmd{
				Torrent: []string{"a.torrent", "b.torrent"},
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
	} {
		var actual cmd
		err := Args(&actual, _case.args)
		if _case.err == nil {
			assert.NoError(t, err)
			assert.EqualValues(t, _case.expected, actual)
		} else if err == nil {
			t.Errorf("expected error: %s", _case.err)
		} else {
			assert.EqualValues(t, _case.err, err)
		}
	}
}

func TestBadCommand(t *testing.T) {
	assert.EqualValues(t,
		userError{"cmd must be ptr or nil"},
		Args(struct{}{}, nil))
	assert.NoError(t, Args(new(struct{}), nil))
	assert.NoError(t, Args(nil, nil))
}
