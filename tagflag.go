package tagflag

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Struct fields after this one are considered positional arguments.
type StartPos struct{}

var GotBuiltinHelpFlag = errors.New("help flag")

func ParseErr(cmd interface{}, args []string, opts ...parseOpt) (err error) {
	p, err := newParser(cmd, opts...)
	if err != nil {
		return
	}
	return p.parse(args)
}

func Parse(cmd interface{}, opts ...parseOpt) {
	err := ParseErr(cmd, os.Args, BuiltinHelp())
	if err == GotBuiltinHelpFlag {
		PrintUsage(cmd, os.Stderr)
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tagflag: %s\n", err)
		if _, ok := err.(userError); ok {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// Turn a struct field name into a flag name. In particular this lower cases
// leading acronyms, and the first capital letter.
func fieldLongFlagKey(fieldName string) (ret string) {
	// defer func() { log.Println(fieldName, ret) }()
	// TCP
	if ss := regexp.MustCompile("^[[:upper:]]{2,}$").FindStringSubmatch(fieldName); ss != nil {
		return strings.ToLower(ss[0])
	}
	// TCPAddr
	if ss := regexp.MustCompile("^([[:upper:]]+)([[:upper:]][^[:upper:]].*?)$").FindStringSubmatch(fieldName); ss != nil {
		return strings.ToLower(ss[1]) + ss[2]
	}
	// Addr
	if ss := regexp.MustCompile("^([[:upper:]])(.*)$").FindStringSubmatch(fieldName); ss != nil {
		return strings.ToLower(ss[1]) + ss[2]
	}
	panic(fieldName)
}
