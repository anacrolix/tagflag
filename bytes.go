package tagflag

import (
	"encoding"

	"github.com/dustin/go-humanize"
)

// A nice builtin type that will marshal human readable byte quantities to
// int64. For example 100GB. See https://godoc.org/github.com/dustin/go-humanize.
type Bytes int64

var (
	_ Marshaler                = (*Bytes)(nil)
	_ encoding.TextUnmarshaler = (*Bytes)(nil)
)

func (me *Bytes) Marshal(s string) (err error) {
	ui64, err := humanize.ParseBytes(s)
	if err != nil {
		return
	}
	*me = Bytes(ui64)
	return
}

func (me *Bytes) UnmarshalText(text []byte) error {
	return me.Marshal(string(text))
}

func (*Bytes) RequiresExplicitValue() bool {
	return false
}

func (me Bytes) Int64() int64 {
	return int64(me)
}

func (me Bytes) String() string {
	return humanize.Bytes(uint64(me))
}
