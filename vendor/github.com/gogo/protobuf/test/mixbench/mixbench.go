// Protocol Buffers for Go with Gadgets
//
// Copyright (c) 2013, The GoGo Authors. All rights reserved.
// http://github.com/gogo/protobuf
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//     * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"fmt"
	"io/ioutil"
	"os/exec"
)

func bench(folder, rgx string, outFileName string) {
	var test = exec.Command("go", "test", "-test.timeout=20m", "-test.v", "-test.run=XXX", "-test.bench="+rgx, folder)
	fmt.Printf("benching\n")
	out, err := test.CombinedOutput()
	fmt.Printf("bench output: %v\n", string(out))
	if err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(outFileName, out, 0666); err != nil {
		panic(err)
	}
}

func main() {
	bench("./test/combos/both/", "ProtoMarshal", "./test/mixbench/marshaler.txt")
	bench("./test/", "ProtoMarshal", "./test/mixbench/marshal.txt")
	bench("./test/combos/both/", "ProtoUnmarshal", "./test/mixbench/unmarshaler.txt")
	bench("./test/", "ProtoUnmarshal", "./test/mixbench/unmarshal.txt")
	fmt.Println("Running benchcmp will show the performance difference between using reflect and generated code for marshalling and unmarshalling of protocol buffers")
	fmt.Println("benchcmp ./test/mixbench/marshal.txt ./test/mixbench/marshaler.txt")
	fmt.Println("benchcmp ./test/mixbench/unmarshal.txt ./test/mixbench/unmarshaler.txt")
}
