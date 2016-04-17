package tagflag

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/bradfitz/iter"
	"github.com/huandu/xstrings"
)

type parser struct {
	cmd           interface{}
	noDefaultHelp bool
	program       string
	description   string

	posArgs []arg
	flags   map[string]arg

	numPos int
}

type arity struct {
	min, max int
}

type arg struct {
	marshal func(argValue string, explicitValue bool) error
	arity   arity
	name    string
	help    string
	_type   reflect.Type
}

func (p *parser) hasOptions() bool {
	return len(p.flags) != 0
}

func (p *parser) parse(args []string) (err error) {
	args, err = p.parseAny(args)
	if err != nil {
		return
	}
	err = p.parsePosArgs(args)
	if err != nil {
		return
	}
	if p.numPos < p.minPos() {
		return userError{fmt.Sprintf("missing argument: %q", p.indexPosArg(p.numPos).name)}
	}
	return
}

func (p *parser) minPos() (min int) {
	for _, arg := range p.posArgs {
		min += arg.arity.min
	}
	return
}

func newParser(cmd interface{}, opts ...parseOpt) (p *parser, err error) {
	p = &parser{
		cmd: cmd,
	}
	for _, opt := range opts {
		opt(p)
	}
	err = p.parseCmd()
	return
}

func (p *parser) parseCmd() error {
	if p.cmd == nil {
		return nil
	}
	s := reflect.ValueOf(p.cmd).Elem()
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct got %s", s.Type())
	}
	return p.parseStruct(reflect.ValueOf(p.cmd).Elem())
}

func canMarshal(f reflect.Value) bool {
	return valueMarshaler(f) != nil
}

// Positional arguments are marked per struct.
func (p *parser) parseStruct(st reflect.Value) (err error) {
	posStarted := false
	foreachStructField(st, func(f reflect.Value, sf reflect.StructField) (stop bool) {
		if !posStarted && f.Type() == reflect.TypeOf(StartPos{}) {
			posStarted = true
			return false
		}
		if canMarshal(f) {
			if posStarted {
				err = p.addPos(f, sf)
			} else {
				err = p.addFlag(f, sf)
				if err != nil {
					err = fmt.Errorf("error adding flag in %s: %s", st.Type(), err)
				}
			}
			return err != nil
		}
		if f.Kind() == reflect.Struct {
			err = p.parseStruct(f)
			return err != nil
		}
		err = fmt.Errorf("field has bad type: %v", f.Type())
		return true
	})
	return
}

func fieldArity(v reflect.Value, sf reflect.StructField) (arity arity) {
	arity.min = 1
	arity.max = 1
	if v.Kind() == reflect.Slice {
		arity.max = infArity
	}
	if sf.Tag.Get("arity") != "" {
		switch sf.Tag.Get("arity") {
		case "?":
			arity.min = 0
		case "*":
			arity.min = 0
			arity.max = infArity
		case "+":
			arity.max = infArity
		default:
			panic(fmt.Sprintf("unhandled arity tag: %q", sf.Tag.Get("arity")))
		}
	}
	return
}

func newArg(v reflect.Value, sf reflect.StructField, name string) arg {
	return arg{
		marshal: valueMarshaler(v),
		arity:   fieldArity(v, sf),
		_type:   v.Type(),
		name:    name,
		help:    sf.Tag.Get("help"),
	}
}

func (p *parser) addPos(f reflect.Value, sf reflect.StructField) error {
	p.posArgs = append(p.posArgs, newArg(f, sf, strings.ToUpper(xstrings.ToSnakeCase(sf.Name))))
	return nil
}

func (p *parser) addFlag(f reflect.Value, sf reflect.StructField) error {
	name := structFieldFlag(sf)
	if _, ok := p.flags[name]; ok {
		return fmt.Errorf("flag %q defined more than once", name)
	}
	if p.flags == nil {
		p.flags = make(map[string]arg)
	}
	p.flags[name] = newArg(f, sf, name)
	return nil
}

func (p *parser) parseAny(args []string) (left []string, err error) {
	for len(args) != 0 {
		a := args[0]
		args = args[1:]
		if a == "--" {
			left = args[1:]
			return
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			err = p.parseFlag(a[1:])
		} else {
			err = p.parsePos(a)
		}
		if err != nil {
			break
		}
	}
	return
}

func (p *parser) parsePosArgs(args []string) (err error) {
	for _, a := range args {
		err = p.parsePos(a)
		if err != nil {
			break
		}
	}
	return
}

func (p *parser) parseFlag(s string) error {
	i := strings.IndexByte(s, '=')
	k := s
	v := ""
	if i != -1 {
		k = s[:i]
		v = s[i+1:]
	}
	flag, ok := p.flags[k]
	if !ok {
		if (k == "help" || k == "h") && !p.noDefaultHelp {
			return ErrDefaultHelp
		}
		return userError{fmt.Sprintf("unknown flag: %q", k)}
	}
	err := flag.marshal(v, i != -1)
	if err != nil {
		return fmt.Errorf("error setting flag %q: %s", k, err)
	}
	return nil
}

func (p *parser) indexPosArg(i int) *arg {
	for _, arg := range p.posArgs {
		if i < arg.arity.max {
			return &arg
		}
		i -= arg.arity.max
	}
	return nil
}

func (p *parser) parsePos(s string) (err error) {
	arg := p.indexPosArg(p.numPos)
	if arg == nil {
		return userError{fmt.Sprintf("excess argument: %q", s)}
	}
	err = arg.marshal(s, true)
	if err != nil {
		return
	}
	p.numPos++
	return
}

func structFieldFlag(sf reflect.StructField) string {
	name := sf.Tag.Get("name")
	if name != "" {
		return name
	}
	return fieldLongFlagKey(sf.Name)
}

func flagValue(value reflect.Value, flag string) (ret reflect.Value) {
	if value.Kind() != reflect.Struct {
		return
	}
	foreachStructField(value, func(fv reflect.Value, sf reflect.StructField) (more bool) {
		if structFieldFlag(sf) == flag {
			ret = fv
			return false
		}
		ret = flagValue(fv, flag)
		if ret.IsValid() {
			return false
		}
		return true
	})
	return
}

func valueMarshaler(v reflect.Value) func(s string, explicitValue bool) error {
	if am, ok := v.Addr().Interface().(Arg); ok {
		return am.Marshal
	}
	if f, ok := typeMarshalFuncs[v.Type()]; ok {
		return func(s string, ev bool) error { return f(v, s, ev) }
	}
	// if f, ok := typeMarshalFuncs[reflect.PtrTo(v.Type())]; ok {
	// 	return f(v.Addr(), s)
	// }
	switch v.Kind() {
	case reflect.Ptr, reflect.Struct:
		return nil
	}
	return func(s string, ev bool) error {
		if v.Kind() == reflect.Bool && s == "" {
			v.SetBool(true)
			return nil
		}
		if v.Kind() == reflect.Slice {
			n := reflect.New(v.Type().Elem())
			err := valueMarshaler(n.Elem())(s, ev)
			if err != nil {
				return err
			}
			v.Set(reflect.Append(v, n.Elem()))
			return nil
		}
		n, err := fmt.Sscan(s, v.Addr().Interface())
		if err != nil {
			return fmt.Errorf("error parsing %q: %s", s, err)
		}
		if n != 1 {
			panic(n)
		}
		return nil
	}
}

// Gets the reflect.Value for the nth positional argument.
func posIndexValue(v reflect.Value, _i int) (ret reflect.Value, i int) {
	i = _i
	log.Println("posIndexValue", v.Type(), i)
	switch v.Kind() {
	case reflect.Ptr:
		return posIndexValue(v.Elem(), i)
	case reflect.Struct:
		posStarted := false
		foreachStructField(v, func(fv reflect.Value, sf reflect.StructField) bool {
			log.Println("posIndexValue struct field", fv, sf)
			if !posStarted {
				if fv.Type() == reflect.TypeOf(StartPos{}) {
					// log.Println("posStarted")
					posStarted = true
				}
				return true
			}
			ret, i = posIndexValue(fv, i)
			if ret.IsValid() {
				return false
			}
			return true
		})
		return
	case reflect.Slice:
		ret = v
		return
	default:
		if i == 0 {
			ret = v
			return
		}
		i--
		return
	}
}

func foreachStructField(_struct reflect.Value, f func(fv reflect.Value, sf reflect.StructField) (stop bool)) {
	t := _struct.Type()
	for i := range iter.N(t.NumField()) {
		sf := t.Field(i)
		fv := _struct.Field(i)
		if f(fv, sf) {
			break
		}
	}
}

func (p *parser) posWithHelp() (ret []arg) {
	for _, a := range p.posArgs {
		if a.help != "" {
			ret = append(ret, a)
		}
	}
	return
}
