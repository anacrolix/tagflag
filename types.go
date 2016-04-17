package tagflag

import (
	"net"
	"net/url"
	"reflect"
	"time"

	"github.com/dustin/go-humanize"
)

type Arg interface {
	Marshal(in string, explicitValue bool) error
}

type Bytes int64

var _ Arg = new(Bytes)

func (me *Bytes) Marshal(s string, _ bool) (err error) {
	ui64, err := humanize.ParseBytes(s)
	if err != nil {
		return
	}
	*me = Bytes(ui64)
	return
}

func (me Bytes) Int64() int64 {
	return int64(me)
}

var typeMarshalFuncs = map[reflect.Type]func(settee reflect.Value, arg string, explicitValue bool) error{}

func addMarshalFunc(f interface{}, explicitValueRequired bool) {
	v := reflect.ValueOf(f)
	t := v.Type()
	setType := t.Out(0)
	typeMarshalFuncs[setType] = func(settee reflect.Value, arg string, explicitValue bool) error {
		if explicitValueRequired && !explicitValue {
			return userError{"explicit value required"}
		}
		out := v.Call([]reflect.Value{reflect.ValueOf(arg)})
		settee.Set(out[0])
		if len(out) > 1 {
			i := out[1].Interface()
			if i != nil {
				return i.(error)
			}
		}
		return nil
	}
}

func init() {
	addMarshalFunc(func(urlStr string) (*url.URL, error) {
		return url.Parse(urlStr)
	}, false)
	addMarshalFunc(func(s string) (*net.TCPAddr, error) {
		return net.ResolveTCPAddr("tcp", s)
	}, true)
	addMarshalFunc(func(s string) (time.Duration, error) {
		return time.ParseDuration(s)
	}, false)
	addMarshalFunc(func(s string) net.IP {
		return net.ParseIP(s)
	}, false)
}
