# Copyright 2011 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

maketables: maketables.go triegen.go
	go build $^

normregtest: normregtest.go
	go build $^

tables:	maketables
	./maketables > tables.go
	gofmt -w tables.go

# Downloads from www.unicode.org, so not part
# of standard test scripts.
test: testtables regtest

testtables: maketables
	./maketables -test > data_test.go && go test -tags=test

regtest: normregtest
	./normregtest
