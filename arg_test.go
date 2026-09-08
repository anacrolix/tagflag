package tagflag

import (
	"net"
	"reflect"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestEqualZeroArgValue(t *testing.T) {
	a := arg{value: reflect.ValueOf(net.IP(nil))}
	qt.Check(t, qt.IsTrue(a.hasZeroValue()))
	b := arg{value: reflect.ValueOf(net.ParseIP("127.0.0.1"))}
	qt.Check(t, qt.IsFalse(b.hasZeroValue()))
}
