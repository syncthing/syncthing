package missinggo

import (
	"fmt"
	"io"
	"runtime"
)

func WriteStack(w io.Writer, stack []uintptr) {
	for _, pc := range stack {
		if pc == 0 {
			break
		}
		pc--
		f := runtime.FuncForPC(pc)
		if f.Name() == "runtime.goexit" {
			continue
		}
		file, line := f.FileLine(pc)
		fmt.Fprintf(w, "# %s:\t%s:%d\n", f.Name(), file, line)
	}
	fmt.Fprintf(w, "\n")
}
