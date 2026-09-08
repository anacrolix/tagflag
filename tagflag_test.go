package tagflag

import (
	"log"
	"net"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/go-quicktest/qt"
	"golang.org/x/xerrors"
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
			simpleCmd{Arg: "hello, world"},
			nil,
			[]string{"hello, world"},
		},
		{
			simpleCmd{},
			userError{`excess argument: "answer = 42"`},
			[]string{"hello, world", "answer = 42"},
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
		qt.Check(t, qt.IsTrue(xerrors.Is(err, _case.err)))
		if _case.err != nil || _case.err != err {
			// The value we got doesn't matter.
			continue
		}
		qt.Check(t, qt.Equals(actual, _case.expected))
	}
}

func TestNotBasic(t *testing.T) {
	t.Skip("outdated use of parseCase")
	type cmd struct {
		Seed       bool
		NoUpload   bool
		ListenAddr string
		DataDir    string `name:"d"`
		StartPos
		Torrent []string `arity:"+"`
	}
	for _, _case := range []parseCase{
		errorCase(userError{`missing argument: "TORRENT"`}, "-seed"),
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
		if _case.err != nil {
			_case.err(t, err)
			continue
		}
		qt.Check(t, qt.IsNil(err))
		qt.Check(t, qt.DeepEquals[any](actual, _case.expected))
	}
}

func TestBadCommand(t *testing.T) {
	// qt.Check(t, qt.IsNotNil(ParseErr(struct{}{}, nil)))
	qt.Check(t, qt.IsNil(ParseErr(new(struct{}), nil)))
	qt.Check(t, qt.IsNil(ParseErr(nil, nil)))
}

func TestVarious(t *testing.T) {
	a := &struct {
		StartPos
		A string `arity:"?"`
	}{}
	qt.Check(t, qt.IsNil(ParseErr(a, nil)))
	qt.Check(t, qt.IsNil(ParseErr(a, []string{"a"})))
	qt.Check(t, qt.Equals(a.A, "a"))
	qt.Check(t, qt.ErrorMatches(
		ParseErr(a, []string{"a", "b"}),
		regexp.QuoteMeta(`excess argument: "b"`)))
}

func TestUint(t *testing.T) {
	var a struct {
		A uint
	}
	qt.Check(t, qt.IsNotNil(ParseErr(&a, []string{"-a"})))
	qt.Check(t, qt.IsNotNil(ParseErr(&a, []string{"-a", "-1"})))
	qt.Check(t, qt.IsNil(ParseErr(&a, []string{"-a=42"})))
}

func TestBasicPositionalArities(t *testing.T) {
	t.Skip("outdated use of parseCase")
	type cmd struct {
		C bool
		StartPos
		A string
		B int64    `arity:"?"`
		D []string `arity:"*"`
	}
	for _, _case := range []parseCase{
		// {nil, userError{`missing argument: "A"`}, cmd{}},
		{[]string{"abc"}, nil, cmd{A: "abc"}},
		{[]string{"abc", "123"}, nil, cmd{A: "abc", B: 123}},
		{[]string{"abc", "123", "first"}, nil, cmd{A: "abc", B: 123, D: []string{"first"}}},
		{[]string{"abc", "123", "first", "second"}, nil, cmd{A: "abc", B: 123, D: []string{"first", "second"}}},
		{[]string{"abc", "123", "-c", "first", "second"}, nil, cmd{A: "abc", B: 123, C: true, D: []string{"first", "second"}}},
	} {
		var actual cmd
		err := ParseErr(&actual, _case.args)
		if _case.err != nil {
			_case.err(t, err)
			continue
		}
		qt.Check(t, qt.IsNil(err))
		qt.Check(t, qt.DeepEquals[any](actual, _case.expected))
	}
}

func TestBytes(t *testing.T) {
	var cmd struct {
		B Bytes
	}
	err := ParseErr(&cmd, []string{"-b=100g"})
	qt.Check(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(cmd.B, 100e9))
}

func TestPtrToCustom(t *testing.T) {
	var cmd struct {
		Addr *net.TCPAddr
	}
	err := ParseErr(&cmd, []string{"-addr=:443"})
	qt.Check(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(cmd.Addr.String(), ":443"))
	err = ParseErr(&cmd, []string{"-addr="})
	qt.Check(t, qt.IsNil(err))
	qt.Check(t, qt.IsNil(cmd.Addr))
}

func TestResolveTCPAddr(t *testing.T) {
	addr, err := net.ResolveTCPAddr("tcp", "")
	t.Log(addr, err)
}

func TestMain(m *testing.M) {
	log.SetFlags(log.Lshortfile)
	os.Exit(m.Run())
}

func TestDefaultLongFlagName(t *testing.T) {
	f := func(expected, start string) {
		qt.Check(t, qt.Equals(string(fieldFlagName(start)), expected), qt.Commentf("%s", start))
	}
	f("noUpload", "NoUpload")
	f("dht", "DHT")
	f("noIPv6", "NoIPv6")
	f("noIpv6", "NoIpv6")
	f("tcpAddr", "TCPAddr")
	f("addr", "Addr")
	f("v", "V")
	f("a", "A")
	f("redisUrl", "RedisURL")
	f("redisUrl", "redisURL")
}

func TestPrintUsage(t *testing.T) {
	err := ParseErr(nil, []string{"-h"})
	qt.Check(t, qt.IsTrue(xerrors.Is(err, ErrDefaultHelp)), qt.Commentf("%#v", err))
	err = ParseErr(nil, []string{"-help"})
	qt.Check(t, qt.IsTrue(xerrors.Is(err, ErrDefaultHelp)))
}

func TestParseUnnamedTypes(t *testing.T) {
	var cmd1 struct {
		A []byte
		B bool
	}
	qt.Check(t, qt.IsNil(ParseErr(&cmd1, nil)))
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
	qt.Assert(t, qt.IsNil(ParseErr(&cmd1, []string{"a", "b", "c"})))
	qt.Check(t, qt.DeepEquals(cmd1.Args, []string{"a", "b", "c"}))
}

func TestTCPAddrNoExplicitValue(t *testing.T) {
	var cmd struct {
		Addr *net.TCPAddr
	}
	qt.Check(t, qt.IsNotNil(ParseErr(&cmd, []string{"-addr"})))
	qt.Check(t, qt.IsNil(ParseErr(&cmd, []string{"-addr="})))
}

func TestUnexportedStructField(t *testing.T) {
	var cmd struct {
		badField bool
	}
	qt.Check(t, qt.IsNil(ParseErr(&cmd, nil)))
	var ue userError
	qt.Assert(t, qt.IsTrue(xerrors.As(ParseErr(&cmd, []string{"-badField"}), &ue)))
	qt.Check(t, qt.Equals(ue, userError{`unknown flag: "badField"`}))
}

func TestExcessArgsEmpty(t *testing.T) {
	var cmd struct {
		ExcessArgs
	}
	qt.Assert(t, qt.IsNil(ParseErr(&cmd, nil)))
	qt.Check(t, qt.HasLen(cmd.ExcessArgs, 0))
}

func TestExcessArgs(t *testing.T) {
	var cmd struct {
		ExcessArgs
	}
	excess := []string{"yo", "-addr=hi"}
	qt.Assert(t, qt.IsNil(ParseErr(&cmd, excess)))
	qt.Check(t, qt.DeepEquals([]string(cmd.ExcessArgs), excess))
}

func TestExcessArgsComplex(t *testing.T) {
	var cmd struct {
		Verbose bool `name:"v"`
		StartPos
		Command string
		ExcessArgs
	}
	excess := []string{"-addr=hi"}
	qt.Assert(t, qt.IsNil(ParseErr(&cmd, append([]string{"-v", "serve"}, excess...))))
	qt.Check(t, qt.DeepEquals([]string(cmd.ExcessArgs), excess))
}

func TestFieldAfterExcessArgs(t *testing.T) {
	var cmd struct {
		ExcessArgs
		Badness string
	}
	qt.Assert(t, qt.Equals(ParseErr(&cmd, nil), ErrFieldsAfterExcessArgs))
}

func TestSliceOfUnmarshallableStruct(t *testing.T) {
	var cmd struct {
		StartPos
		Complex []struct{}
	}
	qt.Assert(t, qt.ErrorMatches(
		ParseErr(&cmd, []string{"herp"}),
		regexp.QuoteMeta("can't marshal type struct {}")))
}

func newStruct(ref interface{}) func() interface{} {
	return func() interface{} {
		return reflect.New(reflect.TypeOf(ref)).Interface()
	}
}

func TestBasicPointer(t *testing.T) {
	type cmd struct {
		Maybe *bool
	}
	_true := true
	RunCases(t, []parseCase{
		noErrorCase(cmd{}),
		errorCase(userError{`excess argument: "nope"`}, "nope"),
		noErrorCase(cmd{Maybe: &_true}, "-maybe=true"),
	}, newStruct(cmd{}))
}

func TestMarshalByteArray(t *testing.T) {
	type cmd struct {
		StartPos
		Bs [2]byte
	}
	RunCases(t, []parseCase{
		noErrorCase(cmd{Bs: func() (ret [2]byte) { copy(ret[:], "AB"); return }()}, "4142"),
		anyErrorCase("41424"),
		// anyErrorCase("4142"),
	}, newStruct(cmd{}))
}

func TestMarshalStruct(t *testing.T) {
	type InternalStruct struct {
		A bool
	}
	var cmd struct {
		Struct InternalStruct
		StartPos
		StructPos InternalStruct
	}
	ParseErr(&cmd, []string{"-struct", "structpos"})
}
