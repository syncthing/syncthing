// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"log"
	"os"
	"runtime"
	"strings"
)

var dbgprint func(...interface{})

var dbgprintf func(string, ...interface{})

var dbgcallstack func(max int) []string

func init() {
	if _, ok := os.LookupEnv("NOTIFY_DEBUG"); ok || debugTag {
		log.SetOutput(os.Stdout)
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		dbgprint = func(v ...interface{}) {
			v = append([]interface{}{"[D] "}, v...)
			log.Println(v...)
		}
		dbgprintf = func(format string, v ...interface{}) {
			format = "[D] " + format
			log.Printf(format, v...)
		}
		dbgcallstack = func(max int) []string {
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
		return
	}
	dbgprint = func(v ...interface{}) {}
	dbgprintf = func(format string, v ...interface{}) {}
	dbgcallstack = func(max int) []string { return nil }
}
