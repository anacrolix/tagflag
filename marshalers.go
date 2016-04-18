package tagflag

import (
	"fmt"
	"reflect"
)

type Marshaler interface {
	Marshal(in string) error
	RequiresExplicitValue() bool
}

type marshaler interface {
	Marshal(reflect.Value, string) error
	RequiresExplicitValue() bool
}

type dynamicMarshaler struct {
	explicitValueRequired bool
	marshal               func(reflect.Value, string) error
}

func (me dynamicMarshaler) Marshal(v reflect.Value, s string) error {
	return me.marshal(v, s)
}

func (me dynamicMarshaler) RequiresExplicitValue() bool {
	return me.explicitValueRequired
}

// The fallback marshaler, that attempts to use fmt.Sscan, and recursion to
// sort marshal types.
type defaultMarshaler struct{}

func (defaultMarshaler) Marshal(v reflect.Value, s string) error {
	if v.Kind() == reflect.Slice {
		n := reflect.New(v.Type().Elem())
		m := valueMarshaler(n.Elem())
		err := m.Marshal(n.Elem(), s)
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

func (defaultMarshaler) RequiresExplicitValue() bool {
	return false
}
