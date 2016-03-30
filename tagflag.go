package tagflag

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

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
			fmt.Fprintf(tw, "  %s\t(%s)\t%s\n", a.name, a.value.Type(), a.help)
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
		fmt.Fprintf(tw, "%s%s", longFlagPrefix, f.long)
		fmt.Fprintf(tw, "\t(%s)\t%s\n", f.value.Type(), f.help)
	}
	tw.Flush()
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
	long string
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

const flagNameTag = "name"

var relevantTags = []string{
	"short", flagNameTag, "help",
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
		raiseLogicError(fmt.Sprintf("can't set field %s, is it exported?", sf.Name))
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
		long: sf.Tag.Get(flagNameTag),
		arg: arg{
			help:  sf.Tag.Get("help"),
			value: v,
			arity: arity(unequalsArity(v.Type())),
		},
	}
	if f.long == "" {
		f.long = fieldLongFlagKey(sf.Name)
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
	posStarted := false
	foreachStructField(v, func(fv reflect.Value, sf reflect.StructField) {
		if fv.Type() == reflect.TypeOf(StartPos{}) {
			posStarted = true
			return
		}
		if posStarted {
			p.addArg(&p.cmd, fv, sf)
		} else {
			p.addFlag(&p.cmd.options, fv, sf)
		}
	})
}

// func (p *parser) addAny(cmd *command, fv reflect.Value, sf reflect.StructField) {
// 	switch sf.Tag.Get("type") {
// 	case "flag":
// 		p.addFlag(&cmd.options, fv, sf)
// 	case "pos":
// 		p.addArg(cmd, fv, sf)
// 	default:
// 		if fv.Kind() == reflect.Struct && unsettableType(fv.Type()) != nil {
// 			name := sf.Tag.Get("name")
// 			if name == "" {
// 				name = sf.Name
// 			}
// 			p.addOptionGroup(p.newOptionGroup(name), fv)
// 		} else {
// 			p.addFlag(&cmd.options, fv, sf)
// 			// p.raiseUserError(fmt.Sprintf("bad type in tag for %s: %q", sf.Name, sf.Tag))
// 		}
// 	}
// }

func (p *parser) newOptionGroup(name string) *optionGroup {
	p.optGroups = append(p.optGroups, optionGroup{name: name})
	return &p.optGroups[len(p.optGroups)-1]
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
	if p.printHelp && (name == "h" || name == "help") {
		p.raisePrintUsage()
	}
	return nil
}

func (p *parser) raiseUnexpectedFlag(flag string) {
	p.raiseUserError(fmt.Sprintf("unexpected flag: %q", flag))
}

var PrintHelp = errors.New("help flag")

func (p *parser) raisePrintUsage() {
	exc.Raise(PrintHelp)
}

const longFlagPrefix = "-"

func (p *parser) parseLongFlag() {
	parts := strings.SplitN(p.next()[len(longFlagPrefix):], "=", 2)
	p.advance()
	key := parts[0]
	f := p.getLongFlag(key)
	if f == nil {
		if p.printHelp && key == "help" {
			p.raisePrintUsage()
		}
		p.raiseUnexpectedFlag(longFlagPrefix + key)
	}
	if len(parts) > 1 {
		n, err := p.setValue(f.arg, parts[1:2], true)
		if err != nil {
			raiseUserError(fmt.Sprintf("error setting %s: %s", longFlagPrefix+key, err))
		}
		if n != 1 {
			panic(n)
		}
	} else {
		n, err := p.setValue(f.arg, p.args, false)
		if err != nil {
			raiseUserError(fmt.Sprintf("error setting %s: %s", longFlagPrefix+key, err))
		}
		p.args = p.args[n:]
	}
}

func (p *parser) parseFlag() {
	p.parseLongFlag()
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

// Apply a custom value function that returns a pointer, to a non-pointer
// value of that type. va is the address of the value to be set.
func setValueAddr(args []string, va reflect.Value, f customSetter) (arity int) {
	fv := reflect.New(va.Type())
	arity, err := f(fv.Elem(), args)
	if err != nil {
		panic(err)
	}
	va.Elem().Set(fv.Elem().Elem())
	return
}

func setValue(args []string, v reflect.Value) int {
	if am, ok := v.Addr().Interface().(ArgsMarshaler); ok {
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
	if st := typeSetters[v.Addr().Type()]; st != nil {
		return setValueAddr(args, v.Addr(), st)
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
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		x, err := strconv.ParseInt(args[0], 0, 64)
		if err != nil {
			raiseUserError(err.Error())
		}
		v.SetInt(x)
		return 1
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		x, err := strconv.ParseUint(args[0], 0, 64)
		if err != nil {
			raiseUserError(err.Error())
		}
		v.SetUint(x)
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

func addTypeSetter(parser interface{}) {
	parserValue := reflect.ValueOf(parser)
	parserType := parserValue.Type()
	setType := parserType.Out(0)
	_, ok := typeSetters[setType]
	if ok {
		panic("already added")
	}
	typeSetters[setType] = func(setValue reflect.Value, args []string) (arity int, err error) {
		var in []reflect.Value
		for i := range iter.N(parserType.NumIn()) {
			in = append(in, reflect.ValueOf(args[i]))
		}
		out := parserValue.Call(in)
		setValue.Set(out[0])
		errInt := out[1].Interface()
		if errInt != nil {
			err = errInt.(error)
		}
		arity = parserType.NumIn()
		return
	}
}

// Takes arguments and sets the value, returning the number of arguments
// consumed, and any parsing error.
type customSetter func(reflect.Value, []string) (int, error)

var typeSetters = map[reflect.Type]func(reflect.Value, []string) (int, error){}

func init() {
	addTypeSetter(func(urlStr string) (*url.URL, error) {
		return url.Parse(urlStr)
	})
	addTypeSetter(func(s string) (*net.TCPAddr, error) {
		return net.ResolveTCPAddr("tcp", s)
	})
	addTypeSetter(func(s string) (time.Duration, error) {
		return time.ParseDuration(s)
	})
}

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
	if typeSetters[reflect.PtrTo(t)] != nil {
		return
	}
	switch t.Kind() {
	case reflect.Bool, reflect.String:
	case reflect.Ptr:
		ret = unsettableType(t.Elem())
	case reflect.Slice:
		ret = unsettableType(t.Elem())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
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
	p := newParser(cmd, append([]parseOpt{
		ExitOnError(),
		HelpFlag(),
		Program(filepath.Base(os.Args[0])),
	}, opts...)...)
	err := p.parse(os.Args[1:])
	if err == PrintHelp {
		p.WriteUsage(os.Stdout)
		os.Exit(0)
	}
	if err != nil {
		panic(err)
	}
}

func newParser(cmd interface{}, parseOpts ...parseOpt) (p *parser) {
	p = &parser{
		errorWriter: os.Stderr,
		program:     "program",
	}
	for _, po := range parseOpts {
		po(p)
	}
	p.addCmd(cmd)
	return
}

// Parse the provided command-line-style arguments, by default returning any
// errors.
func ParseEx(cmd interface{}, args []string, parseOpts ...parseOpt) (err error) {
	p := newParser(cmd, parseOpts...)
	return p.parse(args)
}

func (p *parser) parse(args []string) (err error) {
	p.args = args
	exc.TryCatch(func() {
		for len(p.args) != 0 {
			p.parseAny()
		}
		// TODO: I don't think this is working, add tests.
		p.assertRequiredArgs()
	}, func(e *exc.Exception) {
		if e.Value == PrintHelp {
			err = e.Value.(error)
			return
		}
		switch v := e.Value.(type) {
		case userError, logicError:
			err = v.(error)
		default:
			e.Raise()
		}
	})
	if err == PrintHelp {
		return
	}
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

// Struct fields after this one are considered positional arguments.
type StartPos struct{}
