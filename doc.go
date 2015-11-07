// Package tagflag uses reflection to derive flags and positional arguments to a
// program, and parses and sets them from a slice of arguments.
//
// For example:
//  var opts struct {
//      Mmap           bool           `help:"memory-map torrent data"`
//      TestPeer       []*net.TCPAddr `short:"p" help:"addresses of some starting peers"`
//      Torrent        []string       `type:"pos" arity:"+" help:"torrent file path or magnet uri"`
//  }
//  tagflag.Parse(&opts)
//
// Supported tags include:
//  help: a line of test to show after the option
//  short: a single character for the "-X" form of an option
//  long: an override for the --some-option form derived from the fields name
//  type: defaults to flag. set to pos for positional arguments, that are set
//        from their position in the argument slice ignoring flags and flag values
//  arity: defaults to 1. the number of arguments a field requires, or ? for one
//         optional argument, + for one or more, or * for zero or more.
//
// MarshalArgs is called on fields that implement ArgsMarshaler. A number of
// arguments matching the arity of the field are passed if possible.
package tagflag
