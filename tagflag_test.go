package tagflag

import (
	"log"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	type simpleCmd struct {
		Verbose bool `name:"v"`
		StartPos
		Arg string
	}
	for _, _case := range []struct {
		expected simpleCmd
		err      error
		args     []string
	}{
		{
			simpleCmd{Verbose: true, Arg: "test"},
			nil,
			[]string{"-v", "test"},
		},
		{
			simpleCmd{Verbose: false, Arg: "hello"},
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
			userError{`unknown flag: "no"`},
			[]string{"-no"},
		},
	} {
		var actual simpleCmd
		err := ParseErr(&actual, _case.args)
		assert.EqualValues(t, _case.err, err)
		if _case.err != nil || _case.err != err {
			// The value we got doesn't matter.
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
		DataDir    string `name:"d"`
		StartPos
		Torrent []string `arity:"+"`
	}
	for _, _case := range []struct {
		args     []string
		err      error
		expected cmd
	}{
		{nil, userError{`missing argument: "TORRENT"`}, cmd{}},
		{
			[]string{"-seed"},
			userError{`missing argument: "TORRENT"`},
			cmd{},
		},
		{
			[]string{"-seed", "a.torrent", "b.torrent"},
			nil,
			cmd{
				Torrent: []string{"a.torrent", "b.torrent"},
				Seed:    true,
			},
		},
		{
			[]string{"-listenAddr=1.2.3.4:80", "a.torrent", "b.torrent"},
			nil,
			cmd{
				ListenAddr: "1.2.3.4:80",
				Torrent:    []string{"a.torrent", "b.torrent"},
			},
		},
		{
			[]string{"-d=/tmp", "a.torrent", "b.torrent", "-listenAddr=1.2.3.4:80"},
			nil,
			cmd{
				DataDir:    "/tmp",
				ListenAddr: "1.2.3.4:80",
				Torrent:    []string{"a.torrent", "b.torrent"},
			},
		},
		{
			[]string{"-noUpload=true", "-noUpload=false", "a.torrent"},
			nil,
			cmd{
				NoUpload: false,
				Torrent:  []string{"a.torrent"},
			},
		},
	} {
		var actual cmd
		err := ParseErr(&actual, _case.args)
		assert.EqualValues(t, _case.err, err)
		if _case.err != nil {
			continue
		}
		assert.EqualValues(t, _case.expected, actual)
	}
}

func TestBadCommand(t *testing.T) {
	// assert.Error(t, ParseErr(struct{}{}, nil))
	assert.NoError(t, ParseErr(new(struct{}), nil))
	assert.NoError(t, ParseErr(nil, nil))
}

func TestVarious(t *testing.T) {
	a := &struct {
		StartPos
		A string `arity:"?"`
	}{}
	assert.NoError(t, ParseErr(a, nil))
	assert.NoError(t, ParseErr(a, []string{"a"}))
	assert.EqualValues(t, "a", a.A)
	assert.EqualError(t, ParseErr(a, []string{"a", "b"}), `excess argument: "b"`)
}

func TestUint(t *testing.T) {
	var a struct {
		A uint
	}
	assert.Error(t, ParseErr(&a, []string{"-a"}))
	assert.Error(t, ParseErr(&a, []string{"-a", "-1"}))
	assert.NoError(t, ParseErr(&a, []string{"-a=42"}))
}

func TestBasicPositionalArities(t *testing.T) {
	type cmd struct {
		C bool
		StartPos
		A string
		B int64    `arity:"?"`
		D []string `arity:"*"`
	}
	for _, _case := range []struct {
		args     []string
		err      error
		expected cmd
	}{
		// {nil, userError{`missing argument: "A"`}, cmd{}},
		{[]string{"abc"}, nil, cmd{A: "abc"}},
		{[]string{"abc", "123"}, nil, cmd{A: "abc", B: 123}},
		{[]string{"abc", "123", "first"}, nil, cmd{A: "abc", B: 123, D: []string{"first"}}},
		{[]string{"abc", "123", "first", "second"}, nil, cmd{A: "abc", B: 123, D: []string{"first", "second"}}},
		{[]string{"abc", "123", "-c", "first", "second"}, nil, cmd{A: "abc", B: 123, C: true, D: []string{"first", "second"}}},
	} {
		var actual cmd
		err := ParseErr(&actual, _case.args)
		assert.EqualValues(t, _case.err, err)
		if _case.err != nil {
			continue
		}
		assert.EqualValues(t, _case.expected, actual)
	}
}

func TestBytes(t *testing.T) {
	var cmd struct {
		B Bytes
	}
	err := ParseErr(&cmd, []string{"-b=100g"})
	assert.NoError(t, err)
	assert.EqualValues(t, 100e9, cmd.B)
}

func TestPtrToCustom(t *testing.T) {
	var cmd struct {
		Addr *net.TCPAddr
	}
	err := ParseErr(&cmd, []string{"-addr=:443"})
	assert.NoError(t, err)
	assert.EqualValues(t, ":443", cmd.Addr.String())
}

func TestMain(m *testing.M) {
	log.SetFlags(log.Lshortfile)
	os.Exit(m.Run())
}

func TestDefaultLongFlagName(t *testing.T) {
	assert.EqualValues(t, "noUpload", fieldLongFlagKey("NoUpload"))
	assert.EqualValues(t, "dht", fieldLongFlagKey("DHT"))
	assert.EqualValues(t, "noIPv6", fieldLongFlagKey("NoIPv6"))
	assert.EqualValues(t, "tcpAddr", fieldLongFlagKey("TCPAddr"))
	assert.EqualValues(t, "addr", fieldLongFlagKey("Addr"))
	assert.EqualValues(t, "v", fieldLongFlagKey("V"))
	assert.EqualValues(t, "a", fieldLongFlagKey("A"))
}

func TestPrintUsage(t *testing.T) {
	err := ParseErr(nil, []string{"-h"}, BuiltinHelp())
	assert.Equal(t, GotBuiltinHelpFlag, err)
	err = ParseErr(nil, []string{"-help"}, BuiltinHelp())
	assert.Equal(t, GotBuiltinHelpFlag, err)
}

func TestParseUnnamedTypes(t *testing.T) {
	var cmd1 struct {
		A []byte
		B bool
	}
	assert.NoError(t, ParseErr(&cmd1, nil))
	type B []byte
	var cmd2 struct {
		A B
	}
	ParseErr(&cmd2, nil)
	type C bool
	var cmd3 struct {
		A C
	}
	ParseErr(&cmd3, nil)
}

func TestPosArgSlice(t *testing.T) {
	var cmd1 struct {
		StartPos
		Args []string
	}
	require.NoError(t, ParseErr(&cmd1, []string{"a", "b", "c"}))
	assert.EqualValues(t, []string{"a", "b", "c"}, cmd1.Args)
}
