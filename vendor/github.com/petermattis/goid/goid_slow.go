// Copyright 2016 Peter Mattis.
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

// +build go1.4,!go1.5,!amd64,!amd64p32,!arm,!386 go1.5,!go1.6,!amd64,!amd64p32,!arm go1.6,!amd64,!amd64p32,!arm go1.9,!amd64,!amd64p32,!arm

package goid

// Get returns the id of the current goroutine.
func Get() int64 {
	return getSlow()
}
