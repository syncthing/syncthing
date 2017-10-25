// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build debug

package notify

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

func dbgprint(v ...interface{}) {
	fmt.Printf("[D] ")
	fmt.Print(v...)
	fmt.Printf("\n\n")
}

func dbgprintf(format string, v ...interface{}) {
	fmt.Printf("[D] ")
	fmt.Printf(format, v...)
	fmt.Printf("\n\n")
}

func dbgcallstack(max int) []string {
	pc, stack := make([]uintptr, max), make([]string, 0, max)
	runtime.Callers(2, pc)
	for _, pc := range pc {
		if f := runtime.FuncForPC(pc); f != nil {
			fname := f.Name()
			idx := strings.LastIndex(fname, string(os.PathSeparator))
			if idx != -1 {
				stack = append(stack, fname[idx+1:])
			} else {
				stack = append(stack, fname)
			}
		}
	}
	return stack
}
