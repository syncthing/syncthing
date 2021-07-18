// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"github.com/syncthing/syncthing/lib/config/mocks"
)

func newMockedConfig() *mocks.Wrapper {
	m := &mocks.Wrapper{}
	m.ModifyReturns(noopWaiter{}, nil)
	m.RemoveFolderReturns(noopWaiter{}, nil)
	m.RemoveDeviceReturns(noopWaiter{}, nil)
	return m
}

type noopWaiter struct{}

func (noopWaiter) Wait() {}
