# Copyright 2014 The b Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY: all todo clean cover mem

testbin=b.test

all: editor
	go build
	go vet
	make todo

editor:
	go fmt
	go test -i
	go test

mem:
	go test -c
	./$(testbin) -test.bench . -test.memprofile mem.out -test.memprofilerate 1
	go tool pprof --lines --web --alloc_space $(testbin) mem.out

todo:
	@grep -n ^[[:space:]]*_[[:space:]]*=[[:space:]][[:alpha:]][[:alnum:]]* *.go || true
	@grep -n TODO *.go || true
	@grep -n BUG *.go || true
	@grep -n println *.go || true

clean:
	@go clean
	rm -f *~

cover:
	t=$(shell tempfile) ; go test -coverprofile $$t && go tool cover -html $$t && unlink $$t
