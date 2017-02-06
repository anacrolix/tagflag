package tagflag

import (
	"fmt"
	"reflect"
)

type arg struct {
	arity arity
	name  string
	help  string
	value reflect.Value
}

func (me arg) hasZeroValue() bool {
	return reflect.DeepEqual(
		reflect.Zero(me.value.Type()).Interface(),
		me.value.Interface())
}

func (me arg) marshal(s string, explicitValue bool) error {
	m := valueMarshaler(me.value)
	if !explicitValue && m.RequiresExplicitValue() {
		return userError{fmt.Sprintf("explicit value required (%s%s=VALUE)", flagPrefix, me.name)}
	}
	return valueMarshaler(me.value).Marshal(me.value, s)
}
