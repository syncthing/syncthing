// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package suturewrap

import (
	"context"
	"strings"
	"testing"
)

func TestUtilStopTwicePanic(t *testing.T) {
	name := "foo"
	s := AsService(func(ctx context.Context) {
		<-ctx.Done()
	}, name)

	go s.Serve()
	s.Stop()

	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), name) {
			t.Fatalf(`expected panic containing "%v", got "%v"`, name, r)
		}
	}()
	s.Stop()
}
