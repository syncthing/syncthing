// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ur

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syncthing/syncthing/lib/build"
)

func TestFailuresReadWrite(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "syncthing_testDir-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	f := filepath.Join(tmpDir, "foo")
	l := 10
	reports := make([]FailureReport, l)
	for i := 0; i < l; i++ {
		reports[i].Description = fmt.Sprintf("failure %v", i)
		reports[i].Count = i
		reports[i].Version = build.LongVersion
	}

	if err := writeFailures(f, reports); err != nil {
		t.Fatal(err)
	}

	read, err := readFailures(f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reports, read) {
		t.Errorf("Reports changed after writing and reading:\n%v\n%v", reports, read)
	}
}
