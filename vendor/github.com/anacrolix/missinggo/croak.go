package missinggo

import (
	"fmt"
	"os"
)

func Unchomp(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s
	}
	return s + "\n"
}

func Fatal(msg interface{}) {
	os.Stderr.WriteString(Unchomp(fmt.Sprint(msg)))
	os.Exit(1)
}
