# tagflag

[![GoDoc](https://godoc.org/github.com/anacrolix/tagflag?status.svg)](https://godoc.org/github.com/anacrolix/tagflag)

Declarative flag parsing that uses the type system and tags. Supports positional arguments. Tag features inspired by the parameters allowed in Python's [`argparse.ArgumentParser.add_argument`](https://docs.python.org/3/library/argparse.html#argparse.ArgumentParser.add_argument).
```
var opts struct {
	Mmap           bool           `help:"memory-map torrent data"`
	TestPeer       []*net.TCPAddr `short:"p" help:"addresses of some starting peers"`
	Torrent        []string       `type:"pos" arity:"+" help:"torrent file path or magnet uri"`
}
tagflag.Parse(&opts)
```
Passing `-h` or `--help` to this program gives:
```
Usage:
  torrent [OPTIONS...] TORRENT...
Arguments:
  TORRENT   torrent file path or magnet uri
Options:
  --mmap            memory-map torrent data
  -p, --test-peer   addresses of some starting peers
```
Custom types are supported through use of the `ArgMarshaler` interface.
