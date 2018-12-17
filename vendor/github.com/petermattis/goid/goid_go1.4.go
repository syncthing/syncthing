// Copyright 2015 Peter Mattis.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

// +build go1.4,!go1.5

package goid

import "unsafe"

var pointerSize = unsafe.Sizeof(uintptr(0))

// Backdoor access to runtime·getg().
func getg() uintptr // in goid_go1.4.s

// Get returns the id of the current goroutine.
func Get() int64 {
	// The goid is the 16th field in the G struct where each field is a
	// pointer, uintptr or padded to that size. See runtime.h from the
	// Go sources. I'm not aware of a cleaner way to determine the
	// offset.
	return *(*int64)(unsafe.Pointer(getg() + 16*pointerSize))
}
