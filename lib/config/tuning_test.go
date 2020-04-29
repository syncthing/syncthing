// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config_test

import (
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db/backend"
)

func TestTuningMatches(t *testing.T) {
	if int(config.TuningAuto) != int(backend.TuningAuto) {
		t.Error("mismatch for TuningAuto")
	}
	if int(config.TuningSmall) != int(backend.TuningSmall) {
		t.Error("mismatch for TuningSmall")
	}
	if int(config.TuningLarge) != int(backend.TuningLarge) {
		t.Error("mismatch for TuningLarge")
	}
}
