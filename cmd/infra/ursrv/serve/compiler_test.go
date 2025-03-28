// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import "testing"

func TestCompilerRe(t *testing.T) {
	tests := [][3]string{
		{`syncthing v0.11.0 (xgcc (Ubuntu 4.9.3-0ubuntu4) 4.9.3 linux-amd64 default) niklas@Niklas-Netbook 2015-04-26 13:15:08 UTC`, "xgcc (Ubuntu 4.9.3-0ubuntu4) 4.9.3", "niklas@Niklas-Netbook"},
		{`syncthing v0.12.0-rc5 "Beryllium Bedbug" (go1.4.2 linux-arm android) unknown-user@Felix-T420 2015-10-22 18:32:15 UTC`, "go1.4.2", "unknown-user@Felix-T420"},
		{`syncthing v0.13.0-beta.0+39-ge267bf3 "Copper Cockroach" (go1.4.2 linux-amd64) portage@slevermann.de 2016-01-20 08:41:52 UTC`, "go1.4.2", "portage@slevermann.de"},
	}

	for _, tc := range tests {
		m := compilerRe.FindStringSubmatch(tc[0])
		if len(m) != 3 {
			t.Errorf("Regexp didn't match %q", tc[0])
			continue
		}
		if m[1] != tc[1] {
			t.Errorf("Compiler %q != %q", m[1], tc[1])
		}
		if m[2] != tc[2] {
			t.Errorf("Builder %q != %q", m[2], tc[2])
		}
	}
}
