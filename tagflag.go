package tagflag

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"golang.org/x/xerrors"
)

// Struct fields after this one are considered positional arguments.
type StartPos struct{}

// Default help flag was provided, and should be handled.
var ErrDefaultHelp = errors.New("help flag")

// Parses given arguments, returning any error.
func ParseErr(cmd interface{}, args []string, opts ...parseOpt) (err error) {
	p, err := newParser(cmd, opts...)
	if err != nil {
		return
	}
	return p.parse(args)
}

// Parses the command-line arguments, exiting the process appropriately on
// errors or if usage is printed.
func Parse(cmd interface{}, opts ...parseOpt) *Parser {
	opts = append([]parseOpt{Program(filepath.Base(os.Args[0]))}, opts...)
	return ParseArgs(cmd, os.Args[1:], opts...)
}

// Like Parse, but operates on the given args instead.
func ParseArgs(cmd interface{}, args []string, opts ...parseOpt) *Parser {
	p, err := newParser(cmd, opts...)
	if err == nil {
		err = p.parse(args)
	}
	if xerrors.Is(err, ErrDefaultHelp) {
		p.printUsage(os.Stdout)
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tagflag: error parsing args: %v\n", err)
		if _, ok := err.(userError); ok {
			os.Exit(2)
		}
		os.Exit(1)
	}
	return p
}

func Unmarshal(arg string, v interface{}) error {
	_v := reflect.ValueOf(v).Elem()
	m := valueMarshaler(_v.Type())
	if m == nil {
		return fmt.Errorf("can't unmarshal to type %s", _v.Type())
	}
	return m.Marshal(_v, arg)
}
