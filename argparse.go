package argparse

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/anacrolix/exc"
	"github.com/bradfitz/iter"
	"github.com/huandu/xstrings"
)

type parser struct {
	program     string
	args        []string
	cmd         parsedCmd
	nargs       int
	exitOnError bool
	errorWriter io.Writer
	printHelp   bool
}

func (pc *parsedCmd) WriteUsage(w io.Writer, program string) {
	fmt.Fprintf(w, "Usage: %s [OPTIONS...]", program)
	for _, arg := range pc.args {
		fs := func() string {
			switch arg.arity {
			case '-':
				return "<%s>"
			case '?':
				return "[%s]"
			case '+':
				return "%s..."
			case '*':
				return "[%s...]"
			default:
				panic(arg.arity)
			}
		}()
		fmt.Fprintf(w, " "+fs, arg.name)
	}
	fmt.Fprintf(w, "\n\n")
	if len(pc.flags) == 0 {
		return
	}
	fmt.Fprintf(w, "Options:\n")
	tw := tabwriter.NewWriter(w, 8, 2, 3, ' ', 0)
	for _, f := range pc.flags {
		fmt.Fprint(tw, "  ")
		if f.short != 0 {
			fmt.Fprintf(tw, "-%c, ", f.short)
		}
		fmt.Fprintf(tw, "--%s", f.long)
		fmt.Fprintf(tw, "\t%s\n", f.desc)
	}
	tw.Flush()
}

func (p *parser) PrintUsage() {
	p.cmd.WriteUsage(p.errorWriter, p.program)
}

func (p *parser) parse() {
	for len(p.args) != 0 {
		p.parseAny()
	}
	p.assertRequiredArgs()
}

func (p *parser) assertRequiredArgs() {
	ra := 0
	for _, a := range p.cmd.args {
		switch a.arity {
		case '-':
			ra++
		case '?', '*':
		case '+':
			ra++
		default:
			panic(a.arity)
		}
		if p.nargs < ra {
			p.raiseUserError(fmt.Sprintf("missing argument: %q", a.name))
		}
	}
}

func (p *parser) next() string {
	return p.args[0]
}

type userError struct {
	msg string
}

func (ue userError) Error() string {
	return ue.msg
}

type flag struct {
	value reflect.Value
	short rune
	long  string
	desc  string
}

type arg struct {
	value reflect.Value
	arity byte
	name  string
	desc  string
}

type parsedCmd struct {
	flags []flag
	args  []arg
}

func (pc *parsedCmd) addArg(arity byte, v reflect.Value, name string, desc string) {
	pc.args = append(pc.args, arg{v, arity, name, desc})
}

func fieldLongFlagKey(fieldName string) string {
	ret := xstrings.ToSnakeCase(fieldName)
	return strings.Replace(ret, "_", "-", -1)
}

func (p *parser) parseCmd(cmd interface{}) (pc parsedCmd) {
	if cmd == nil {
		return
	}
	v := reflect.ValueOf(cmd)
	if v.Kind() != reflect.Ptr {
		p.raiseUserError(fmt.Sprintf("cmd must be ptr or nil"))
	}
	v = v.Elem()
	t := v.Type()
	for i := range iter.N(t.NumField()) {
		sf := t.Field(i)
		fv := v.Field(i)
		tag := sf.Tag.Get("argparse")
		tagParts := strings.SplitN(tag, ":", 2)
		var desc string
		if len(tagParts) > 1 {
			desc = tagParts[1]
		}
		cfg := tagParts[0]
		switch cfg {
		case "-", "?", "+", "*":
			pc.addArg(cfg[0], fv, fieldLongFlagKey(sf.Name), desc)
			continue
		case "":
			pc.flags = append(pc.flags, flag{
				value: fv,
				long:  fieldLongFlagKey(sf.Name),
				desc:  desc,
			})
		default:
			if len(cfg) == 2 && cfg[0] == '-' {
				f := flag{
					value: fv,
					long:  fieldLongFlagKey(sf.Name),
					desc:  desc,
				}
				f.short, _ = utf8.DecodeRuneInString(cfg[1:])
				pc.flags = append(pc.flags, f)
			} else {
				p.raiseUserError(fmt.Sprintf("bad flag tag: %q", tag))
			}
		}
	}
	return
}

func (p *parser) getShortFlag(name rune) *flag {
	for i := range p.cmd.flags {
		f := &p.cmd.flags[i]
		if f.short == name {
			return f
		}
	}
	return nil
}

func (p *parser) getLongFlag(name string) *flag {
	for _, f := range p.cmd.flags {
		if f.long == name {
			return &f
		}
	}
	return nil
}

func (p *parser) raiseUnexpectedFlag(flag string) {
	p.raiseUserError(fmt.Sprintf("unexpected flag: %q", flag))
}

type printHelp struct{}

func (p *parser) raisePrintUsage() {
	exc.Raise(printHelp{})
}

func (p *parser) parseLongFlag() {
	f := p.getLongFlag(p.next()[2:])
	if f == nil {
		if p.printHelp && p.next()[2:] == "help" {
			p.raisePrintUsage()
		}
		p.raiseUnexpectedFlag(p.next())
	}
	if f.value.Kind() == reflect.Bool {
		f.value.SetBool(true)
	} else {
		p.advance()
		p.setValue(f.value)
	}
}

func (p *parser) parseShortFlags() {
	for i, c := range p.next()[1:] {
		f := p.getShortFlag(c)
		if f == nil {
			if p.printHelp && c == 'h' {
				p.raisePrintUsage()
			}
			p.raiseUnexpectedFlag(p.next())
		}
		if f.value.Kind() == reflect.Bool {
			f.value.SetBool(true)
		} else if i == len(p.next())-2 {
			p.advance()
			p.setValue(f.value)
		} else {
			p.raiseUserError(fmt.Sprintf("%q in %q wants argument", c, p.next()))
		}
	}
}

func (p *parser) parseFlag() {
	if strings.HasPrefix(p.next(), "--") {
		p.parseLongFlag()
	} else if strings.HasPrefix(p.next(), "-") {
		p.parseShortFlags()
	}
	p.advance()
}

func (p *parser) advance() {
	p.args = p.args[1:]
}

func (p *parser) parseFlagValue(flag flag) {
	switch flag.value.Kind() {
	case reflect.Bool:
		flag.value.SetBool(true)
	default:
		panic(fmt.Sprintf("unhandled flag type: %s", flag.value))
	}
}

func (p *parser) parseAny() {
	if strings.HasPrefix(p.next(), "-") {
		p.parseFlag()
	} else {
		p.parseArg()
	}
}

func (p *parser) raiseUserError(msg string) {
	exc.Raise(userError{msg})
}

func (p *parser) argValue() reflect.Value {
	na := 0
	p.nargs++
	for _, arg := range p.cmd.args {
		na++
		switch arg.arity {
		case '-', '?':
			if na != p.nargs {
				continue
			}
		case '+', '*':
		default:
			panic(arg.arity)
		}
		return arg.value
	}
	p.raiseUserError(fmt.Sprintf("excess argument: %q", p.next()))
	panic("unreachable")
}

func argToKind(arg string, kind reflect.Kind) reflect.Value {
	switch kind {
	case reflect.String:
		return reflect.ValueOf(arg)
	default:
		panic(fmt.Sprintf("unsupported value type: %s", kind))
	}
}

func (p *parser) setValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Slice:
		v.Set(reflect.Append(v, argToKind(p.next(), v.Type().Elem().Kind())))
	default:
		v.Set(argToKind(p.next(), v.Kind()))
	}
}

func (p *parser) parseArg() {
	p.setValue(p.argValue())
	p.nargs++
	p.advance()
}

func Argv(cmd interface{}) {
	err := Args(cmd, os.Args[1:], ExitOnError(), HelpFlag(), Program(filepath.Base(os.Args[0])))
	if err != nil {
		panic(err)
	}
}

type parseOpt func(p *parser)

func ExitOnError() parseOpt {
	return func(p *parser) {
		p.exitOnError = true
	}
}

func HelpFlag() parseOpt {
	return func(p *parser) {
		p.printHelp = true
	}
}

func Program(program string) parseOpt {
	return func(p *parser) {
		p.program = program
	}
}

func Args(cmd interface{}, args []string, parseOpts ...parseOpt) (err error) {
	p := parser{
		args:        args,
		errorWriter: os.Stderr,
		program:     "program",
	}
	for _, po := range parseOpts {
		po(&p)
	}
	exc.TryCatch(func() {
		p.cmd = p.parseCmd(cmd)
		p.parse()
	}, func(e *exc.Exception) {
		// log.Printf("%#v", e)
		ue, ok := e.Value.(userError)
		if ok {
			err = ue
			return
		}
		if _, ok := e.Value.(printHelp); ok {
			p.PrintUsage()
			os.Exit(0)
		}
		e.Raise()
	})
	if err != nil && p.exitOnError {
		fmt.Fprintf(p.errorWriter, "%s\n\n", err)
		p.PrintUsage()
		os.Exit(2)
	}
	return
}
