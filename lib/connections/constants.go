// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

const (
	tcpPriority   = 10
	kcpPriority   = 50
	relayPriority = 200

	kcpNoFilterPriority           = 100
	kcpConversationFilterPriority = 20
	kcpStunFilterPriority         = 10

	kcpNoDelay           = 1
	kcpInterval          = 10
	kcpResend            = 2
	kcpNoCongestion      = 1
	kcpKeepAlive         = 10
	kcpSendWindowSize    = 128
	kcpReceiveWindowSize = 128
)
