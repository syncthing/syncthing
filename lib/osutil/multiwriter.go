// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"errors"
	"io"
)

// TolerantMultiWriter writes to all of its writers, like io.MultiWriter,
// except that a failing write to one writer does not prevent writing to the
// others.
type TolerantMultiWriter []io.Writer

func (t TolerantMultiWriter) Write(p []byte) (int, error) {
	n := len(p)
	var errs []error
	for _, w := range t {
		if _, err := w.Write(p); err != nil {
			errs = append(errs, err)
		}
	}
	return n, errors.Join(errs...)
}
