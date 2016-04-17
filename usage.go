package tagflag

import (
	"fmt"
	"io"
)

func PrintUsage(cmd interface{}, w io.Writer) {
	fmt.Fprintln(w, "usage")
}
