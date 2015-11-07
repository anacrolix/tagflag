package tagflag

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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

func posWithDesc(poss []pos) (ret []pos) {
	for _, a := range poss {
		if a.help != "" {
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
			case arityZeroOrOne:
				return "[%s]"
			case arityOneOrMore:
				return "%s..."
			case arityZeroOrMore:
				return "[%s...]"
			default:
				return "<%s>"
			}
		}()
		if arg.arity != 0 {
			fmt.Fprintf(w, " "+fs, arg.name)
		}
		if arg.arity > 1 {
			for range iter.N(int(arg.arity - 1)) {
				fmt.Fprintf(w, " "+fs, arg.name)
			}
		}
	}
	fmt.Fprintf(w, "\n")
	if p.description != "" {
		fmt.Fprintf(w, "\n%s\n", missinggo.Unchomp(p.description))
	}
	if awd := posWithDesc(p.cmd.args); len(awd) != 0 {
		fmt.Fprintf(w, "Arguments:\n")
		tw := newUsageTabwriter(w)
		for _, a := range awd {
			fmt.Fprintf(tw, "  %s\t%s\n", a.name, a.help)
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
		fmt.Fprintf(tw, "\t%s\n", f.help)
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
		case arityZeroOrMore, arityZeroOrOne:
		case arityOneOrMore:
			ra++
		default:
			ra += int(a.arity)
		}
		if p.nargs < ra {
			p.raiseUserError(fmt.Sprintf("missing argument: %q", a.name))
		}
	}
}

func (p *parser) next() string {
	return p.args[0]
}

type flag struct {
	arg
	short byte
	long  string
}

const (
	arityOneOrMore  = -1
	arityZeroOrMore = -2
	arityZeroOrOne  = -3
)

type arity int

type arg struct {
	value reflect.Value
	arity arity
	help  string
}

type pos struct {
	arg
	name string
}

type command struct {
	options optionGroup
	args    []pos
}

func fieldLongFlagKey(fieldName string) string {
	ret := xstrings.ToSnakeCase(fieldName)
	return strings.Replace(ret, "_", "-", -1)
}

type optionGroup struct {
	name  string
	flags []flag
}

func parseArityString(s string) (a arity, err error) {
	switch s {
	case "*":
		a = arityZeroOrMore
	case "+":
		a = arityOneOrMore
	case "?":
		a = arityZeroOrOne
	default:
		var ui uint64
		ui, err = strconv.ParseUint(s, 10, 0)
		a = arity(ui)
	}
	return
}

func (p *parser) addArg(cmd *command, v reflect.Value, sf reflect.StructField) {
	arg := pos{
		arg: arg{
			value: v,
			help:  sf.Tag.Get("help"),
		},
		name: sf.Tag.Get("name"),
	}
	arity := sf.Tag.Get("arity")
	if arity == "" {
		arity = "1"
	}
	if len(arity) != 1 {
		p.raiseUserError(fmt.Sprintf("bad arity in tag: %q", sf.Tag))
	}
	var err error
	arg.arity, err = parseArityString(arity)
	if err != nil {
		raiseLogicError(fmt.Sprintf("bad arity string %q: %s", arity, err))
	}
	if arg.name == "" {
		arg.name = strings.ToUpper(xstrings.ToSnakeCase(sf.Name))
	}
	cmd.args = append(cmd.args, arg)
}

var relevantTags = []string{
	"short", "long", "help",
}

func hasRelevantTags(sf reflect.StructField) bool {
	for _, rt := range relevantTags {
		if sf.Tag.Get(rt) != "" {
			return true
		}
	}
	return false
}

func (p *parser) skipField(sf reflect.StructField) bool {
	if p.skipUnsettable && !hasRelevantTags(sf) {
		return true
	}
	if sf.Tag.Get("tagflag") == "skip" {
		return true
	}
	return false
}

func (p *parser) skipCannotSet(v reflect.Value, sf reflect.StructField) bool {
	if !v.CanSet() {
		if p.skipField(sf) {
			return true
		}
		raiseLogicError(fmt.Sprintf("can't set field %s", sf.Name))
	}
	t := unsettableType(v.Type())
	if t == nil {
		return false
	}
	if p.skipField(sf) {
		return true
	}
	raiseLogicError(fmt.Sprintf("can't marshal to field %s of type %s", sf.Name, fullTypeName(t)))
	panic("unreachable")
}

func (p *parser) addFlag(g *optionGroup, v reflect.Value, sf reflect.StructField) {
	if p.skipCannotSet(v, sf) {
		return
	}
	f := flag{
		long: sf.Tag.Get("long"),
		arg: arg{
			help:  sf.Tag.Get("help"),
			value: v,
			arity: arity(unequalsArity(v.Type())),
		},
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
	case "pos":
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
	parts := strings.SplitN(p.next()[2:], "=", 2)
	p.advance()
	key := parts[0]
	f := p.getLongFlag(key)
	if f == nil {
		if p.printHelp && key == "help" {
			p.raisePrintUsage()
		}
		p.raiseUnexpectedFlag("--" + key)
	}
	if len(parts) > 1 {
		n, err := p.setValue(f.arg, parts[1:2], true)
		if err != nil {
			raiseUserError(fmt.Sprintf("error setting %s: %s", "--"+key, err))
		}
		if n != 1 {
			panic(n)
		}
	} else {
		n, err := p.setValue(f.arg, p.args, false)
		if err != nil {
			raiseUserError(fmt.Sprintf("error setting %s: %s", "--"+key, err))
		}
		p.args = p.args[n:]
	}
}

func (p *parser) parseShortFlags() {
	next := p.next()[1:]
	p.advance()
	for i := range next {
		c := next[i]
		f := p.getShortFlag(c)
		if f == nil {
			if p.printHelp && c == 'h' {
				p.raisePrintUsage()
			}
			p.raiseUnexpectedFlag("-" + string(c))
		}
		if i == len(next)-1 {
			n, err := p.setValue(f.arg, p.args, false)
			if err != nil {
				raiseUserError(fmt.Sprintf("-%c: %s", c, err))
			}
			p.args = p.args[n:]
			break
		} else {
			p.setValue(f.arg, nil, false)
		}
	}
}

func (p *parser) parseFlag() {
	if strings.HasPrefix(p.next(), "--") {
		p.parseLongFlag()
	} else if strings.HasPrefix(p.next(), "-") {
		p.parseShortFlags()
	}
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
	// log.Println("parsing", p.next())
	if strings.HasPrefix(p.next(), "-") {
		p.parseFlag()
	} else {
		p.parseArg()
	}
}

func raiseUserError(msg string) {
	exc.Raise(userError{msg})
}

func raiseLogicError(msg string) {
	exc.Raise(logicError{msg})
}

func (p *parser) raiseUserError(msg string) {
	raiseUserError(msg)
}

func (p *parser) argPos() pos {
	argPos := 0
	for _, arg := range p.cmd.args {
		switch arg.arity {
		case arityZeroOrOne:
			argPos++
		case arityOneOrMore, arityZeroOrMore:
			argPos = math.MaxInt32
		default:
			argPos += int(arg.arity)
		}
		if argPos > p.nargs {
			return arg
		}
	}
	p.raiseUserError(fmt.Sprintf("excess argument: %q", p.next()))
	panic("unreachable")
}

type ArgsMarshaler interface {
	// Called with arguments based on the field's arity. May be passed exactly
	// one argument in for example the case where a flag is called in the
	// --option=value style. Should return the number of arguments actually
	// consumed.
	MarshalArgs(args []string) (n int, err error)
}

func setValue(args []string, v reflect.Value) int {
	if am, ok := v.Interface().(ArgsMarshaler); ok {
		n, err := am.MarshalArgs(args)
		if err != nil {
			raiseUserError(fmt.Sprintf("error marshaling args: %s", err))
		}
		return n
	}
	if f := typeSetters[v.Type()]; f != nil {
		n, err := f(v, args)
		if err != nil {
			raiseUserError(fmt.Sprintf("error marshaling args: %s", err))
		}
		return n
	}
	switch v.Type().Kind() {
	case reflect.String:
		v.SetString(args[0])
		return 1
	case reflect.Slice:
		x := reflect.New(v.Type().Elem())
		n := setValue(args, x.Elem())
		v.Set(reflect.Append(v, x.Elem()))
		return n
	case reflect.Bool:
		b := true
		switch len(args) {
		case 0:
		case 1:
			var err error
			b, err = strconv.ParseBool(args[0])
			if err != nil {
				raiseUserError(err.Error())
			}
		default:
			raiseUserError(fmt.Sprintf("bad argument count to bool: %d", len(args)))
		}
		v.SetBool(b)
		return len(args)
	case reflect.Int64:
		x, err := strconv.ParseInt(args[0], 0, 64)
		if err != nil {
			raiseUserError(err.Error())
		}
		v.SetInt(x)
		return 1
	default:
		panic(v)
	}
}

func fullTypeName(t reflect.Type) string {
	if t.PkgPath() == "" {
		return t.Name()
	}
	return fmt.Sprintf(`"%s".%s`, t.PkgPath(), t.Name())
}

// Returns number of args consumed.
func (p *parser) setValue(arg arg, args []string, equalsValue bool) (n int, err error) {
	if !equalsValue {
		switch arg.arity {
		case arityOneOrMore, arityZeroOrMore:
		case arityZeroOrOne:
			if len(args) > 1 {
				args = args[:1]
			}
		default:
			if len(args) > int(arg.arity) {
				args = args[:arg.arity]
			}
		}
		switch arg.arity {
		case arityZeroOrOne, arityZeroOrMore:
		case arityOneOrMore:
			if len(args) < 1 {
				err = fmt.Errorf("expected one or more arguments")
				return
			}
		default:
			if len(args) != int(arg.arity) {
				err = fmt.Errorf("expected %d arguments", arg.arity)
				return
			}
		}
	}
	n = setValue(args, arg.value)
	return
}

var (
	typeSetters = map[reflect.Type]func(reflect.Value, []string) (int, error){
		reflect.TypeOf(&net.TCPAddr{}): func(v reflect.Value, args []string) (n int, err error) {
			ta, err := net.ResolveTCPAddr("tcp", args[0])
			if err != nil {
				return
			}
			v.Set(reflect.ValueOf(ta))
			return 1, nil
		},
	}
)

func unequalsArity(t reflect.Type) int {
	if t.Kind() == reflect.Bool {
		return 0
	}
	return 1
}

func unsettableType(t reflect.Type) (ret reflect.Type) {
	if typeSetters[t] != nil {
		return
	}
	switch t.Kind() {
	case reflect.Bool, reflect.String:
		return
	case reflect.Ptr:
		ret = unsettableType(t.Elem())
	case reflect.Slice:
		ret = unsettableType(t.Elem())
	case reflect.Int64:
	default:
		ret = t
	}
	return
}

func (p *parser) parseArg() {
	pos := p.argPos()
	n, err := p.setValue(pos.arg, p.args, false)
	if err != nil {
		raiseUserError(fmt.Sprintf("%s: %s", pos.name, err))
	}
	p.nargs += n
	p.args = p.args[n:]
}

// Parse os.Args, with implicit help flag, program name, and exiting on all
// errors.
func Parse(cmd interface{}, opts ...parseOpt) {
	err := ParseEx(cmd, os.Args[1:], append([]parseOpt{
		ExitOnError(), HelpFlag(), Program(filepath.Base(os.Args[0])),
	}, opts...)...)
	if err != nil {
		panic(err)
	}
}

// Parse the provided command-line-style arguments, by default returning any
// errors.
func ParseEx(cmd interface{}, args []string, parseOpts ...parseOpt) (err error) {
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
		switch v := e.Value.(type) {
		case userError, logicError:
			err = v.(error)
		case printHelp:
			p.WriteUsage(os.Stdout)
			os.Exit(0)
		default:
			e.Raise()
		}
	})
	if err != nil && p.exitOnError {
		fmt.Fprintf(p.errorWriter, "tagflag: %s\n", err)
		code := func() int {
			if _, ok := err.(userError); ok {
				return 2
			} else {
				return 1
			}
		}()
		os.Exit(code)
	}
	return
}
