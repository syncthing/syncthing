// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package versioner

type Versioner interface {
	Archive(path string) error
}

var Factories = map[string]func(map[string]string) Versioner{}
