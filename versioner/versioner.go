// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package versioner implements common interfaces for file versioning and a
// simple default versioning scheme.
package versioner

type Versioner interface {
	Archive(filePath string) error
}

var Factories = map[string]func(repoID string, repoDir string, params map[string]string) Versioner{}
