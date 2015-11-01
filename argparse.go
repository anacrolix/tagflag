package argparse

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/anacrolix/exc"
	"github.com/anacrolix/missinggo"
	"github.com/bradfitz/iter"
	"github.com/huandu/xstrings"
)

type parser struct {
	program   string
	args      []string
	cmd       command
	optGroups []optionGroup
	nargs     int

	exitOnError    bool
	errorWriter    io.Writer
	printHelp      bool
	skipUnsettable bool
	description    string
}

func argsWithDesc(args []arg) (ret []arg) {
	for _, a := range args {
		if a.desc != "" {
			ret = append(ret, a)
		}
	}
	return
}

func (p *parser) hasOptions() bool {
	if len(p.cmd.options.flags) != 0 {
		return true
	}
	for _, og := range p.optGroups {
		if len(og.flags) != 0 {
			return true
		}
	}
	return false
}

func (p *parser) WriteUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage:\n  %s", p.program)
	if p.hasOptions() {
		fmt.Fprintf(w, " [OPTIONS...]")
	}
	for _, arg := range p.cmd.args {
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
	fmt.Fprintf(w, "\n")
	if p.description != "" {
		fmt.Fprintf(w, "\n%s\n", missinggo.Unchomp(p.description))
	}
	if awd := argsWithDesc(p.cmd.args); len(awd) != 0 {
		fmt.Fprintf(w, "Arguments:\n")
		tw := newUsageTabwriter(w)
		for _, a := range awd {
			fmt.Fprintf(tw, "  %s\t%s\n", a.name, a.desc)
		}
		tw.Flush()
	}
	p.writeOptionGroupUsage(w, &p.cmd.options)
	for i := range p.optGroups {
		g := &p.optGroups[i]
		p.writeOptionGroupUsage(w, g)
	}
}

func newUsageTabwriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 8, 2, 3, ' ', 0)
}

func (p *parser) writeOptionGroupUsage(w io.Writer, g *optionGroup) {
	if len(g.flags) == 0 {
		return
	}
	if g.name != "" {
		fmt.Fprintf(w, "%s ", g.name)
	}
	fmt.Fprintf(w, "Options:\n")
	tw := newUsageTabwriter(w)
	for _, f := range g.flags {
		fmt.Fprint(tw, "  ")
		if f.short != 0 {
			fmt.Fprintf(tw, "-%c, ", f.short)
		}
		fmt.Fprintf(tw, "--%s", f.long)
		fmt.Fprintf(tw, "\t%s\n", f.desc)
	}
	tw.Flush()
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
		case '1':
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
	short byte
	long  string
	desc  string
}

type arg struct {
	value reflect.Value
	arity byte
	name  string
	desc  string
}

type command struct {
	options optionGroup
	args    []arg
}

func fieldLongFlagKey(fieldName string) string {
	ret := xstrings.ToSnakeCase(fieldName)
	return strings.Replace(ret, "_", "-", -1)
}

type optionGroup struct {
	name  string
	flags []flag
}

func (p *parser) addArg(cmd *command, v reflect.Value, sf reflect.StructField) {
	arg := arg{
		value: v,
		name:  sf.Tag.Get("name"),
		desc:  sf.Tag.Get("desc"),
	}
	arity := sf.Tag.Get("arity")
	if arity == "" {
		arity = "1"
	}
	if len(arity) != 1 {
		p.raiseUserError(fmt.Sprintf("bad arity in tag: %q", sf.Tag))
	}
	arg.arity = arity[0]
	if arg.name == "" {
		arg.name = strings.ToUpper(xstrings.ToSnakeCase(sf.Name))
	}
	cmd.args = append(cmd.args, arg)
}

func (p *parser) skipCannotSet(t reflect.Type) bool {
	cst := cannotSet(t)
	if cst == nil {
		return false
	}
	if p.skipUnsettable {
		return true
	}
	raiseUserError(fmt.Sprintf("can't set type %s: %s", fullTypeName(cst), cst))
	panic("unreachable")
}

func (p *parser) addFlag(g *optionGroup, v reflect.Value, sf reflect.StructField) {
	if p.skipCannotSet(v.Type()) {
		return
	}
	f := flag{
		long:  sf.Tag.Get("long"),
		desc:  sf.Tag.Get("desc"),
		value: v,
	}
	short := sf.Tag.Get("short")
	switch len(short) {
	case 0:
	case 1:
		f.short = short[0]
	default:
		p.raiseUserError(fmt.Sprintf("bad short tag: %q", sf.Tag))
	}
	if f.long == "" {
		f.long = strings.Replace(xstrings.ToSnakeCase(sf.Name), "_", "-", -1)
	}
	g.flags = append(g.flags, f)
}

func foreachStructField(_struct reflect.Value, f func(fv reflect.Value, sf reflect.StructField)) {
	t := _struct.Type()
	for i := range iter.N(t.NumField()) {
		sf := t.Field(i)
		fv := _struct.Field(i)
		f(fv, sf)
	}
}

func (p *parser) addOptionGroup(g *optionGroup, v reflect.Value) {
	foreachStructField(v, func(fv reflect.Value, sf reflect.StructField) {
		p.addFlag(g, fv, sf)
	})
}

func (p *parser) addCmd(cmd interface{}) {
	if cmd == nil {
		return
	}
	v := reflect.ValueOf(cmd)
	if v.Kind() != reflect.Ptr {
		p.raiseUserError(fmt.Sprintf("cmd must be ptr or nil"))
	}
	v = v.Elem()
	foreachStructField(v, func(fv reflect.Value, sf reflect.StructField) {
		p.addAny(&p.cmd, fv, sf)
	})
}

func (p *parser) addAny(cmd *command, fv reflect.Value, sf reflect.StructField) {
	switch sf.Tag.Get("type") {
	case "flag":
		p.addFlag(&cmd.options, fv, sf)
	case "arg":
		p.addArg(cmd, fv, sf)
	default:
		if fv.Kind() == reflect.Struct {
			name := sf.Tag.Get("name")
			if name == "" {
				name = sf.Name
			}
			p.addOptionGroup(p.newOptionGroup(name), fv)
		} else {
			p.addFlag(&cmd.options, fv, sf)
			// p.raiseUserError(fmt.Sprintf("bad type in tag for %s: %q", sf.Name, sf.Tag))
		}
	}
}

func (p *parser) newOptionGroup(name string) *optionGroup {
	p.optGroups = append(p.optGroups, optionGroup{name: name})
	return &p.optGroups[len(p.optGroups)-1]
}

func (p *parser) getShortFlag(name byte) *flag {
	for i := range p.cmd.options.flags {
		f := &p.cmd.options.flags[i]
		if f.short == name {
			return f
		}
	}
	return nil
}

func (p *parser) getLongFlag(name string) *flag {
	for _, f := range p.cmd.options.flags {
		if f.long == name {
			return &f
		}
	}
	for _, og := range p.optGroups {
		for _, f := range og.flags {
			if f.long == name {
				return &f
			}
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
	for i := range p.next()[1:] {
		c := p.next()[1:][i]
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

func raiseUserError(msg string) {
	exc.Raise(userError{msg})
}

func (p *parser) raiseUserError(msg string) {
	raiseUserError(msg)
}

func (p *parser) argValue() reflect.Value {
	na := 0
	p.nargs++
	for _, arg := range p.cmd.args {
		na++
		switch arg.arity {
		case '1', '?':
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

func argToKind(arg string, kind reflect.Kind) (ret reflect.Value) {
	switch kind {
	case reflect.String:
		ret = reflect.ValueOf(arg)
	}
	return
}

func fullTypeName(t reflect.Type) string {
	if t.PkgPath() == "" {
		return t.Name()
	}
	return fmt.Sprintf(`"%s".%s`, t.PkgPath(), t.Name())
}

func (p *parser) setValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Slice:
		v.Set(reflect.Append(v, argToKind(p.next(), v.Type().Elem().Kind())))
	case reflect.Ptr:
		if v.IsNil() {
			nv := reflect.New(v.Type().Elem())
			p.setValue(nv.Elem())
			v.Set(nv)
		} else {
			p.setValue(v.Elem())
		}
	default:
		x := argToKind(p.next(), v.Kind())
		if !x.IsValid() {
			raiseUserError(fmt.Sprintf("can't convert %q to type %s", p.next(), fullTypeName(v.Type())))
		}
		v.Set(x)
	}
}

func cannotSet(t reflect.Type) (ret reflect.Type) {
	switch t.Kind() {
	case reflect.Bool, reflect.String:
		return
	case reflect.Ptr:
		ret = cannotSet(t.Elem())
	case reflect.Slice:
		ret = cannotSet(t.Elem())
	default:
		ret = t
	}
	return
}

func (p *parser) parseArg() {
	p.setValue(p.argValue())
	p.nargs++
	p.advance()
}

func Argv(cmd interface{}, opts ...parseOpt) {
	err := Args(cmd, os.Args[1:], append([]parseOpt{
		ExitOnError(), HelpFlag(), Program(filepath.Base(os.Args[0])),
	}, opts...)...)
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

func SkipBadTypes() parseOpt {
	return func(p *parser) {
		p.skipUnsettable = true
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

func Description(desc string) parseOpt {
	return func(p *parser) {
		p.description = desc
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
		p.addCmd(cmd)
		p.parse()
	}, func(e *exc.Exception) {
		// log.Printf("%#v", e)
		ue, ok := e.Value.(userError)
		if ok {
			err = ue
			return
		}
		if _, ok := e.Value.(printHelp); ok {
			p.WriteUsage(os.Stdout)
			os.Exit(0)
		}
		e.Raise()
	})
	if err != nil && p.exitOnError {
		fmt.Fprintf(p.errorWriter, "%s\n", err)
		os.Exit(2)
	}
	return
}
