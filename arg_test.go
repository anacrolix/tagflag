package tagflag

import (
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEqualZeroArgValue(t *testing.T) {
	a := arg{value: reflect.ValueOf(net.IP(nil))}
	assert.True(t, a.hasZeroValue())
	b := arg{value: reflect.ValueOf(net.ParseIP("127.0.0.1"))}
	assert.False(t, b.hasZeroValue())
}
