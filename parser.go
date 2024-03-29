package tagflag

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/anacrolix/missinggo/v2/slices"
	"github.com/huandu/xstrings"
	"golang.org/x/xerrors"
)

type Parser struct {
	// The value from which the Parser is built, and values are assigned.
	cmd interface{}
	// Disables the default handling of -h and -help.
	noDefaultHelp bool
	program       string
	description   string
	// Whether the first non-option argument requires that all further arguments are to be treated
	// as positional.
	parseIntermixed bool
	// The Parser that preceded this one, such as in sub-command relationship.
	parent *Parser

	posArgs []arg
	// Maps -K=V to map[K]arg(V)
	flags  map[string]arg
	excess *ExcessArgs

	// Count of positional arguments parsed so far. Used to locate the next
	// positional argument where it's non-trivial (non-unity arity).
	numPos int
}

func (p *Parser) hasOptions() bool {
	return len(p.flags) != 0
}

func (p *Parser) parse(args []string) (err error) {
	posOnly := false
	for len(args) != 0 {
		if p.excess != nil && p.nextPosArg() == nil {
			*p.excess = args
			return
		}
		a := args[0]
		args = args[1:]
		if !posOnly && a == "--" {
			posOnly = true
			continue
		}
		if !posOnly && isFlag(a) {
			err = p.parseFlag(a[1:])
			if err != nil {
				err = xerrors.Errorf("parsing flag %q: %w", a[1:], err)
			}
		} else {
			err = p.parsePos(a)
			if !p.parseIntermixed {
				posOnly = true
			}
		}
		if err != nil {
			return
		}
	}
	if p.numPos < p.minPos() {
		return userError{fmt.Sprintf("missing argument: %q", p.indexPosArg(p.numPos).name)}
	}
	return
}

func (p *Parser) minPos() (min int) {
	for _, arg := range p.posArgs {
		min += arg.arity.min
	}
	return
}

func newParser(cmd interface{}, opts ...parseOpt) (p *Parser, err error) {
	p = &Parser{
		cmd:             cmd,
		parseIntermixed: true,
	}
	for _, opt := range opts {
		opt(p)
	}
	err = p.parseCmd()
	return
}

func (p *Parser) parseCmd() error {
	if p.cmd == nil {
		return nil
	}
	s := reflect.ValueOf(p.cmd).Elem()
	for s.Kind() == reflect.Interface {
		s = s.Elem()
	}
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct got %s", s.Type())
	}
	return p.parseStruct(s, nil)
}

// Positional arguments are marked per struct.
func (p *Parser) parseStruct(st reflect.Value, path []flagNameComponent) (err error) {
	posStarted := false
	foreachStructField(st, func(f reflect.Value, sf reflect.StructField) (stop bool) {
		if !posStarted && f.Type() == reflect.TypeOf(StartPos{}) {
			posStarted = true
			return false
		}
		if f.Type() == reflect.TypeOf(ExcessArgs{}) {
			p.excess = f.Addr().Interface().(*ExcessArgs)
			return false
		}
		if sf.PkgPath != "" {
			return false
		}
		if p.excess != nil {
			err = ErrFieldsAfterExcessArgs
			return true
		}
		if canMarshal(f) {
			if posStarted {
				err = p.addPos(f, sf, path)
			} else {
				err = p.addFlag(f, sf, path)
				if err != nil {
					err = fmt.Errorf("error adding flag in %s: %s", st.Type(), err)
				}
			}
			return err != nil
		}
		var parsed bool
		parsed, err = p.parseEmbeddedStruct(f, sf, path)
		if err != nil {
			err = fmt.Errorf("parsing embedded struct: %w", err)
			stop = true
			return
		}
		if parsed {
			return false
		}
		err = fmt.Errorf("field has bad type: %v", f.Type())
		return true
	})
	return
}

func (p *Parser) parseEmbeddedStruct(f reflect.Value, sf reflect.StructField, path []flagNameComponent) (parsed bool, err error) {
	if f.Kind() == reflect.Ptr {
		f = f.Elem()
	}
	if f.Kind() != reflect.Struct {
		return
	}
	if canMarshal(f.Addr()) {
		err = fmt.Errorf("field %q has type %s, but %s is marshalable", sf.Name, f.Type(), f.Addr().Type())
		return
	}
	parsed = true
	if !sf.Anonymous {
		path = append(path, structFieldFlagNameComponent(sf))
	}
	err = p.parseStruct(f, path)
	return
}

func newArg(v reflect.Value, sf reflect.StructField, name string) arg {
	return arg{
		arity: fieldArity(v, sf),
		value: v,
		name:  name,
		help:  sf.Tag.Get("help"),
	}
}

func (p *Parser) addPos(f reflect.Value, sf reflect.StructField, path []flagNameComponent) error {
	p.posArgs = append(p.posArgs, newArg(f, sf, strings.ToUpper(xstrings.ToSnakeCase(sf.Name))))
	return nil
}

func flagName(comps []flagNameComponent) string {
	var ss []string
	slices.MakeInto(&ss, comps)
	return strings.Join(ss, ".")
}

func (p *Parser) addFlag(f reflect.Value, sf reflect.StructField, path []flagNameComponent) error {
	name := flagName(append(path, structFieldFlagNameComponent(sf)))
	if _, ok := p.flags[name]; ok {
		return fmt.Errorf("flag %q defined more than once", name)
	}
	if p.flags == nil {
		p.flags = make(map[string]arg)
	}
	p.flags[name] = newArg(f, sf, name)
	return nil
}

func isFlag(arg string) bool {
	return len(arg) > 1 && arg[0] == '-'
}

func (p *Parser) parseFlag(s string) error {
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
		return xerrors.Errorf("parsing value %q for flag %q: %w", v, k, err)
	}
	return nil
}

func (p *Parser) indexPosArg(i int) *arg {
	for _, arg := range p.posArgs {
		if i < arg.arity.max {
			return &arg
		}
		i -= arg.arity.max
	}
	return nil
}

func (p *Parser) nextPosArg() *arg {
	return p.indexPosArg(p.numPos)
}

func (p *Parser) parsePos(s string) (err error) {
	arg := p.nextPosArg()
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

type flagNameComponent string

func structFieldFlagNameComponent(sf reflect.StructField) flagNameComponent {
	name := sf.Tag.Get("name")
	if name != "" {
		return flagNameComponent(name)
	}
	return fieldFlagName(sf.Name)
}

func (p *Parser) posWithHelp() (ret []arg) {
	for _, a := range p.posArgs {
		if a.help != "" {
			ret = append(ret, a)
		}
	}
	return
}
