// Copyright 2013, Cong Ding. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// author: Cong Ding <dinggnu@gmail.com>

package main

import (
	"flag"
	"fmt"

	"github.com/ccding/go-stun/stun"
)

func main() {
	var serverAddr = flag.String("s", stun.DefaultServerAddr, "STUN server address")
	var v = flag.Bool("v", false, "verbose mode")
	var vv = flag.Bool("vv", false, "double verbose mode (includes -v)")
	var vvv = flag.Bool("vvv", false, "triple verbose mode (includes -v and -vv)")
	flag.Parse()

	// Creates a STUN client. NewClientWithConnection can also be used if
	// you want to handle the UDP listener by yourself.
	client := stun.NewClient()
	// The default addr (stun.DefaultServerAddr) will be used unless we
	// call SetServerAddr.
	client.SetServerAddr(*serverAddr)
	// Non verbose mode will be used by default unless we call
	// SetVerbose(true) or SetVVerbose(true).
	client.SetVerbose(*v || *vv || *vvv)
	client.SetVVerbose(*vv || *vvv)
	// Discover the NAT and return the result.
	nat, host, err := client.Discover()
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("NAT Type:", nat)
	if host != nil {
		fmt.Println("External IP Family:", host.Family())
		fmt.Println("External IP:", host.IP())
		fmt.Println("External Port:", host.Port())
	}
}
