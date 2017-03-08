// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"time"

	"github.com/xtaci/smux"
)

const (
	tcpPriority   = 10
	kcpPriority   = 50
	relayPriority = 200

	// KCP filter priorities
	kcpNoFilterPriority           = 100
	kcpConversationFilterPriority = 20
	kcpStunFilterPriority         = 10
)

var (
	smuxConfig = &smux.Config{
		KeepAliveInterval: 10 * time.Second,
		KeepAliveTimeout:  30 * time.Second,
		MaxFrameSize:      4096,
		MaxReceiveBuffer:  4 * 1024 * 1024,
	}
)
