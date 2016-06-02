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
// Author: Cong Ding <dinggnu@gmail.com>

package stun

import (
	"net"
	"strconv"
)

type Host struct {
	family uint16
	ip     string
	port   uint16
}

func (h *Host) Family() uint16 {
	return h.family
}

func (h *Host) IP() string {
	return h.ip
}

func (h *Host) Port() uint16 {
	return h.port
}

func (h *Host) TransportAddr() string {
	return net.JoinHostPort(h.ip, strconv.Itoa(int(h.port)))
}
