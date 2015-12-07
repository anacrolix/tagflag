package tagflag

import (
	"github.com/dustin/go-humanize"
)

type Bytes int64

var _ ArgsMarshaler = new(Bytes)

func (me *Bytes) MarshalArgs(in []string) (consumed int, err error) {
	ui64, err := humanize.ParseBytes(in[0])
	if err != nil {
		return
	}
	*me = Bytes(ui64)
	consumed = 1
	return
}

func (me Bytes) Int64() int64 {
	return int64(me)
}
