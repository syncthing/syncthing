package missinggo

import (
	"regexp"
	"runtime"
)

// It will be the one and only identifier after a package specifier.
var testNameRegexp = regexp.MustCompile(`\.(Test[\p{L}_\p{N}]*)$`)

// Returns the name of the test function from the call stack. See
// http://stackoverflow.com/q/35535635/149482 for another method.
func GetTestName() string {
	pc := make([]uintptr, 32)
	n := runtime.Callers(0, pc)
	for i := 0; i < n; i++ {
		name := runtime.FuncForPC(pc[i]).Name()
		ms := testNameRegexp.FindStringSubmatch(name)
		if ms == nil {
			continue
		}
		return ms[1]
	}
	panic("test name could not be recovered")
}
